package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prave/FrogLLM/autosetup"
	"github.com/prave/FrogLLM/event"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	PROFILE_SPLIT_CHAR = ":"
)

type ProxyManager struct {
	sync.Mutex

	config     Config
	configPath string // Path to the config file
	ginEngine  *gin.Engine

	// logging
	proxyLogger    *LogMonitor
	upstreamLogger *LogMonitor
	muxLogger      *LogMonitor

	metricsMonitor *MetricsMonitor

	downloadManager *DownloadManager

	processGroups map[string]*ProcessGroup

	// shutdown signaling
	shutdownCtx    context.Context
	shutdownCancel context.CancelFunc

	// subscription canceller for download progress
	downloadSubCancel context.CancelFunc

	// debounce timer for auto reconfigure after downloads
	autoReconfigTimer *time.Timer
}

func New(config Config) *ProxyManager {
	// set up loggers
	stdoutLogger := NewLogMonitorWriter(os.Stdout)
	upstreamLogger := NewLogMonitorWriter(stdoutLogger)
	proxyLogger := NewLogMonitorWriter(stdoutLogger)

	if config.LogRequests {
		proxyLogger.Warn("LogRequests configuration is deprecated. Use logLevel instead.")
	}

	switch strings.ToLower(strings.TrimSpace(config.LogLevel)) {
	case "debug":
		proxyLogger.SetLogLevel(LevelDebug)
		upstreamLogger.SetLogLevel(LevelDebug)
	case "info":
		proxyLogger.SetLogLevel(LevelInfo)
		upstreamLogger.SetLogLevel(LevelInfo)
	case "warn":
		proxyLogger.SetLogLevel(LevelWarn)
		upstreamLogger.SetLogLevel(LevelWarn)
	case "error":
		proxyLogger.SetLogLevel(LevelError)
		upstreamLogger.SetLogLevel(LevelError)
	default:
		proxyLogger.SetLogLevel(LevelInfo)
		upstreamLogger.SetLogLevel(LevelInfo)
	}

	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	// Set up download directory
	downloadDir := config.DownloadDir
	if downloadDir == "" {
		downloadDir = "./downloads"
	}

	pm := &ProxyManager{
		config:     config,
		configPath: "config.yaml", // Default path, can be overridden
		ginEngine:  gin.New(),

		proxyLogger:    proxyLogger,
		muxLogger:      stdoutLogger,
		upstreamLogger: upstreamLogger,

		metricsMonitor:  NewMetricsMonitor(&config, "config.yaml"),
		downloadManager: NewDownloadManager(downloadDir, proxyLogger),

		processGroups: make(map[string]*ProcessGroup),

		shutdownCtx:    shutdownCtx,
		shutdownCancel: shutdownCancel,
	}

	// create the process groups
	for groupID := range config.Groups {
		processGroup := NewProcessGroup(groupID, config, proxyLogger, upstreamLogger)
		pm.processGroups[groupID] = processGroup
	}

	pm.setupGinEngine()

	// No automatic config modifications on startup - keep it clean and predictable

	// Subscribe to download completion to add folder to DB and auto-regenerate config
	pm.downloadSubCancel = event.On(func(e DownloadProgressEvent) {
		if e.Info != nil && e.Info.Status == StatusCompleted {
			go pm.handleDownloadCompleted(e.Info.FilePath)
		}
	})

	// run any startup hooks
	if len(config.Hooks.OnStartup.Preload) > 0 {
		// do it in the background, don't block startup -- not sure if good idea yet
		go func() {
			discardWriter := &DiscardWriter{}
			for _, realModelName := range config.Hooks.OnStartup.Preload {
				proxyLogger.Infof("Preloading model: %s", realModelName)
				processGroup, _, err := pm.swapProcessGroup(realModelName)

				if err != nil {
					event.Emit(ModelPreloadedEvent{
						ModelName: realModelName,
						Success:   false,
					})
					proxyLogger.Errorf("Failed to preload model %s: %v", realModelName, err)
					continue
				} else {
					req, _ := http.NewRequest("GET", "/", nil)
					processGroup.ProxyRequest(realModelName, discardWriter, req)
					event.Emit(ModelPreloadedEvent{
						ModelName: realModelName,
						Success:   true,
					})
				}
			}
		}()
	}

	return pm
}

// SetConfigPath sets the path to the configuration file
func (pm *ProxyManager) SetConfigPath(path string) {
	pm.configPath = path
}

// quotePath properly quotes file paths that contain spaces or special characters
func (pm *ProxyManager) quotePath(path string) string {
	// Always quote paths that contain spaces (common in external drives like "T7 Shield")
	if strings.Contains(path, " ") {
		// Escape any existing quotes and wrap in quotes
		escaped := strings.ReplaceAll(path, "\"", "\\\"")
		return fmt.Sprintf("\"%s\"", escaped)
	}
	return path
}

// handleDownloadCompleted ensures the downloaded file's folder is tracked, then regenerates config
func (pm *ProxyManager) handleDownloadCompleted(downloadedFilePath string) {
	pm.Lock()
	defer pm.Unlock()
	// Derive folder from file path
	absFile, err := filepath.Abs(downloadedFilePath)
	if err != nil {
		pm.proxyLogger.Warnf("Failed to resolve downloaded file path: %v", err)
		return
	}
	folderPath := filepath.Dir(absFile)

	// Update model folder database if folder is not already present
	if err := pm.updateModelFolderDatabase([]string{folderPath}, true); err != nil {
		pm.proxyLogger.Warnf("Failed to update model folder database for %s: %v", folderPath, err)
		// Continue anyway to try regenerate
	} else {
		pm.proxyLogger.Infof("Added/updated model folder in DB: %s", folderPath)
	}

	// Skip auto-regeneration after manual downloads to preserve model IDs
	// The auto-regeneration was overwriting our carefully crafted model IDs
	// (e.g., bartowski-mistral-22b-v0.1-gguf-q5_k -> mistral-22b-v01-2b)
	// Manual downloads already add models correctly via reloadConfigForNewModel
	pm.proxyLogger.Debug("Skipping auto-regeneration after download to preserve model IDs")
}

// generateConfigFromDBLocked performs full regenerate using saved settings.
// Caller must hold pm.Lock().
func (pm *ProxyManager) generateConfigFromDBLocked() {
	// Load persisted settings; fallback to defaults if missing
	options := autosetup.SetupOptions{
		EnableJinja:      true,
		ThroughputFirst:  true,
		MinContext:       16384,
		PreferredContext: 32768,
	}
	if s, err := pm.loadSystemSettings(); err == nil && s != nil {
		options.EnableJinja = s.EnableJinja
		options.ThroughputFirst = s.ThroughputFirst
		if s.PreferredContext > 0 {
			options.PreferredContext = s.PreferredContext
		}
		if s.RAMGB > 0 {
			options.ForceRAM = s.RAMGB
		}
		if s.VRAMGB > 0 {
			options.ForceVRAM = s.VRAMGB
		}
		if s.Backend != "" {
			options.ForceBackend = s.Backend
		}
	}

	db, err := pm.loadModelFolderDatabase()
	if err != nil {
		pm.proxyLogger.Warnf("Failed to load folder DB for auto-reconfigure: %v", err)
		return
	}
	var folderPaths []string
	for _, f := range db.Folders {
		if f.Enabled {
			folderPaths = append(folderPaths, f.Path)
		}
	}
	if len(folderPaths) == 0 {
		pm.proxyLogger.Warnf("Auto-reconfigure skipped: no enabled folders in DB")
		return
	}

	// Collect models from all folders
	var allModels []autosetup.ModelInfo
	for _, p := range folderPaths {
		models, err := autosetup.DetectModelsWithOptions(p, options)
		if err != nil {
			pm.proxyLogger.Warnf("Folder scan failed (%s): %v", p, err)
			continue
		}
		allModels = append(allModels, models...)
	}
	if len(allModels) == 0 {
		pm.proxyLogger.Warnf("Auto-reconfigure skipped: no models found in tracked folders")
		return
	}

	system := autosetup.DetectSystem()
	_ = autosetup.EnhanceSystemInfo(&system)
	binariesDir := filepath.Join(".", "binaries")
	binary, err := autosetup.DownloadBinary(binariesDir, system, options.ForceBackend)
	if err != nil {
		pm.proxyLogger.Warnf("Auto-reconfigure failed to ensure binary: %v", err)
		return
	}
	generator := autosetup.NewConfigGenerator(folderPaths[0], binary.Path, "config.yaml", options)
	generator.SetSystemInfo(&system)
	generator.SetAvailableVRAM(system.TotalVRAMGB)
	if err := generator.GenerateConfig(allModels); err != nil {
		pm.proxyLogger.Warnf("Auto-reconfigure failed to generate config: %v", err)
		return
	}
	pm.proxyLogger.Info("Auto-restarting after model download and config regeneration")
	event.Emit(ConfigFileChangedEvent{ReloadingState: ReloadingStateStart})
}

func (pm *ProxyManager) setupGinEngine() {
	pm.ginEngine.Use(func(c *gin.Context) {
		// Start timer
		start := time.Now()

		// capture these because /upstream/:model rewrites them in c.Next()
		clientIP := c.ClientIP()
		method := c.Request.Method
		path := c.Request.URL.Path

		// Process request
		c.Next()

		// Stop timer
		duration := time.Since(start)

		statusCode := c.Writer.Status()
		bodySize := c.Writer.Size()

		pm.proxyLogger.Infof("Request %s \"%s %s %s\" %d %d \"%s\" %v",
			clientIP,
			method,
			path,
			c.Request.Proto,
			statusCode,
			bodySize,
			c.Request.UserAgent(),
			duration,
		)
	})

	// see: issue: #81, #77 and #42 for CORS issues
	// respond with permissive OPTIONS for any endpoint
	pm.ginEngine.Use(func(c *gin.Context) {
		if c.Request.Method == "OPTIONS" {
			c.Header("Access-Control-Allow-Origin", "*")
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")

			// allow whatever the client requested by default
			if headers := c.Request.Header.Get("Access-Control-Request-Headers"); headers != "" {
				sanitized := SanitizeAccessControlRequestHeaderValues(headers)
				c.Header("Access-Control-Allow-Headers", sanitized)
			} else {
				c.Header(
					"Access-Control-Allow-Headers",
					"Content-Type, Authorization, Accept, X-Requested-With",
				)
			}
			c.Header("Access-Control-Max-Age", "86400")
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	mm := MetricsMiddleware(pm)

	// Auth middleware for OpenAI-compatible endpoints (optional based on settings)
	auth := pm.requireAPIKey()

	// Set up routes using the Gin engine
	pm.ginEngine.POST("/v1/chat/completions", auth, mm, pm.proxyOAIHandler)
	// Support legacy /v1/completions api, see issue #12
	pm.ginEngine.POST("/v1/completions", auth, mm, pm.proxyOAIHandler)

	// Support embeddings and reranking
	pm.ginEngine.POST("/v1/embeddings", auth, mm, pm.proxyOAIHandler)

	// llama-server's /reranking endpoint + aliases
	pm.ginEngine.POST("/reranking", auth, mm, pm.proxyOAIHandler)
	pm.ginEngine.POST("/rerank", auth, mm, pm.proxyOAIHandler)
	pm.ginEngine.POST("/v1/rerank", auth, mm, pm.proxyOAIHandler)
	pm.ginEngine.POST("/v1/reranking", auth, mm, pm.proxyOAIHandler)

	// llama-server's /infill endpoint for code infilling
	pm.ginEngine.POST("/infill", auth, mm, pm.proxyOAIHandler)

	// llama-server's /completion endpoint
	pm.ginEngine.POST("/completion", auth, mm, pm.proxyOAIHandler)

	// Support audio/speech endpoint
	pm.ginEngine.POST("/v1/audio/speech", auth, pm.proxyOAIHandler)
	pm.ginEngine.POST("/v1/audio/transcriptions", auth, pm.proxyOAIPostFormHandler)

	pm.ginEngine.GET("/v1/models", auth, pm.listModelsHandler)
	pm.ginEngine.GET("/v1/models/search", auth, pm.apiV1SearchModels)  // NEW: Unified model search
	pm.ginEngine.POST("/v1/models/load", auth, pm.apiV1LoadModel)      // NEW: Load model with auto-unload
	pm.ginEngine.POST("/v1/models/unload", auth, pm.apiV1UnloadModel)  // NEW: Unload specific model
	pm.ginEngine.GET("/v1/models/loaded", auth, pm.apiV1GetLoadedModels) // NEW: Get loaded models

	// Info endpoint to show model-to-port mappings
	pm.ginEngine.GET("/info", auth, pm.infoHandler)

	// GPU stats endpoint
	pm.ginEngine.GET("/api/gpu/stats", auth, pm.gpuStatsHandler)

	// in proxymanager_loghandlers.go
	pm.ginEngine.GET("/logs", pm.sendLogsHandlers)
	pm.ginEngine.GET("/logs/stream", pm.streamLogsHandler)
	pm.ginEngine.GET("/logs/stream/:logMonitorID", pm.streamLogsHandler)

	/**
	 * User Interface Endpoints
	 */
	pm.ginEngine.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/ui")
	})

	pm.ginEngine.GET("/upstream", func(c *gin.Context) {
		c.Redirect(http.StatusFound, "/ui/models")
	})
	pm.ginEngine.Any("/upstream/*upstreamPath", pm.proxyToUpstream)

	pm.ginEngine.GET("/unload", pm.unloadAllModelsHandler)
	pm.ginEngine.GET("/running", pm.listRunningProcessesHandler)
	pm.ginEngine.GET("/health", func(c *gin.Context) {
		c.String(http.StatusOK, "OK")
	})

	pm.ginEngine.GET("/favicon.ico", func(c *gin.Context) {
		if data, err := reactStaticFS.ReadFile("ui_dist/favicon.ico"); err == nil {
			c.Data(http.StatusOK, "image/x-icon", data)
		} else {
			c.String(http.StatusInternalServerError, err.Error())
		}
	})

	pm.ginEngine.GET("/apple-touch-icon.png", func(c *gin.Context) {
		if data, err := reactStaticFS.ReadFile("ui_dist/apple-touch-icon.png"); err == nil {
			c.Data(http.StatusOK, "image/png", data)
		} else {
			c.String(http.StatusInternalServerError, err.Error())
		}
	})

	reactFS, err := GetReactFS()
	if err != nil {
		pm.proxyLogger.Errorf("Failed to load React filesystem: %v", err)
	} else {

		// serve files that exist under /ui/*
		pm.ginEngine.StaticFS("/ui", reactFS)

		// server SPA for UI under /ui/*
		pm.ginEngine.NoRoute(func(c *gin.Context) {
			if !strings.HasPrefix(c.Request.URL.Path, "/ui") {
				c.AbortWithStatus(http.StatusNotFound)
				return
			}

			file, err := reactFS.Open("index.html")
			if err != nil {
				c.String(http.StatusInternalServerError, err.Error())
				return
			}
			defer file.Close()
			http.ServeContent(c.Writer, c.Request, "index.html", time.Now(), file)

		})
	}

	// see: proxymanager_api.go
	// add API handler functions
	addApiHandlers(pm)

	// Disable console color for testing
	gin.DisableConsoleColor()
}

// ServeHTTP implements http.Handler interface
func (pm *ProxyManager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	pm.ginEngine.ServeHTTP(w, r)
}

// requireAPIKey returns a gin.HandlerFunc that enforces API key only if enabled in settings.
func (pm *ProxyManager) requireAPIKey() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Allow unauthenticated access to settings endpoint so users can configure a key
		if strings.HasPrefix(c.Request.URL.Path, "/api/settings/system") {
			c.Next()
			return
		}

		if settings, _ := pm.loadSystemSettings(); settings != nil && settings.RequireAPIKey {
			key := c.GetHeader("Authorization")
			if key == "" {
				key = c.GetHeader("X-API-Key")
			}
			if key == "" {
				// Allow API key via query param for EventSource and limited clients
				key = c.Query("api_key")
			}
			if strings.HasPrefix(strings.ToLower(key), "bearer ") {
				key = strings.TrimSpace(key[7:])
			}
			if strings.TrimSpace(key) == "" || strings.TrimSpace(settings.APIKey) == "" || key != settings.APIKey {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "API key required or invalid"})
				return
			}
		}
		c.Next()
	}
}

// StopProcesses acquires a lock and stops all running upstream processes.
// This is the public method safe for concurrent calls.
// Unlike Shutdown, this method only stops the processes but doesn't perform
// a complete shutdown, allowing for process replacement without full termination.
func (pm *ProxyManager) StopProcesses(strategy StopStrategy) {
	pm.Lock()
	defer pm.Unlock()

	// stop Processes in parallel
	var wg sync.WaitGroup
	for _, processGroup := range pm.processGroups {
		wg.Add(1)
		go func(processGroup *ProcessGroup) {
			defer wg.Done()
			processGroup.StopProcesses(strategy)
		}(processGroup)
	}

	wg.Wait()
}

// Shutdown stops all processes managed by this ProxyManager
func (pm *ProxyManager) Shutdown() {
	pm.Lock()
	defer pm.Unlock()

	pm.proxyLogger.Debug("Shutdown() called in proxy manager")

	// Save activity stats before shutting down
	if pm.metricsMonitor != nil && pm.metricsMonitor.ActivityStats != nil {
		if err := pm.metricsMonitor.ActivityStats.SaveToFile(); err != nil {
			pm.proxyLogger.Errorf("Failed to save activity stats on shutdown: %v", err)
		} else {
			pm.proxyLogger.Debug("Activity stats saved on shutdown")
		}
	}

	var wg sync.WaitGroup
	// Send shutdown signal to all process in groups
	for _, processGroup := range pm.processGroups {
		wg.Add(1)
		go func(processGroup *ProcessGroup) {
			defer wg.Done()
			processGroup.Shutdown()
		}(processGroup)
	}
	wg.Wait()
	if pm.downloadSubCancel != nil {
		pm.downloadSubCancel()
		pm.downloadSubCancel = nil
	}
	pm.shutdownCancel()
}

func (pm *ProxyManager) swapProcessGroup(requestedModel string) (*ProcessGroup, string, error) {
	// de-alias the real model name and get a real one
	realModelName, found := pm.config.RealModelName(requestedModel)
	if !found {
		return nil, realModelName, fmt.Errorf("could not find real modelID for %s", requestedModel)
	}

	pm.proxyLogger.Debugf("swapProcessGroup: requestedModel=%s, realModelName=%s", requestedModel, realModelName)

	processGroup := pm.findGroupByModelName(realModelName)
	if processGroup == nil {
		// Log available process groups for debugging
		pm.proxyLogger.Warnf("Could not find process group for model %s (real name: %s)", requestedModel, realModelName)
		pm.proxyLogger.Debugf("Available process groups:")
		for groupName, group := range pm.processGroups {
			pm.proxyLogger.Debugf("  Group %s has members: %v", groupName, group.processes)
		}
		return nil, realModelName, fmt.Errorf("could not find process group for model %s", requestedModel)
	}

	// Check memory before loading a new model
	if err := pm.ensureMemoryAvailable(processGroup, realModelName); err != nil {
		return nil, realModelName, fmt.Errorf("memory check failed: %v", err)
	}

	if processGroup.exclusive {
		pm.proxyLogger.Debugf("Exclusive mode for group %s, stopping other process groups", processGroup.id)
		for groupId, otherGroup := range pm.processGroups {
			if groupId != processGroup.id && !otherGroup.persistent {
				otherGroup.StopProcesses(StopWaitForInflightRequest)
			}
		}
	}

	return processGroup, realModelName, nil
}

func (pm *ProxyManager) listModelsHandler(c *gin.Context) {
	data := make([]gin.H, 0, len(pm.config.Models))
	createdTime := time.Now().Unix()

	for id, modelConfig := range pm.config.Models {
		if modelConfig.Unlisted {
			continue
		}

		record := gin.H{
			"id":       id,
			"object":   "model",
			"created":  createdTime,
			"owned_by": "FrogLLM",
		}

		if name := strings.TrimSpace(modelConfig.Name); name != "" {
			record["name"] = name
		}
		if desc := strings.TrimSpace(modelConfig.Description); desc != "" {
			record["description"] = desc
		}

		// Extract model path from Cmd if available
		modelPath := ""
		if modelConfig.Cmd != "" {
			// Extract model path from llama-server command
			cmdParts := strings.Fields(modelConfig.Cmd)
			for i, part := range cmdParts {
				if part == "-m" && i+1 < len(cmdParts) {
					modelPath = cmdParts[i+1]
					break
				}
			}
		}

		// Add enhanced model information
		if modelPath != "" {
			// Check if model file exists and get size
			if fileInfo, err := os.Stat(modelPath); err == nil {
				sizeGB := float64(fileInfo.Size()) / (1024 * 1024 * 1024)
				record["size_gb"] = fmt.Sprintf("%.2f", sizeGB)
				record["size_bytes"] = fileInfo.Size()
				record["file_exists"] = true
			} else {
				record["file_exists"] = false
			}
			record["model_path"] = modelPath
		}

		// Add quantization info if available in the path/name
		if strings.Contains(modelPath, "Q4_K_M") || strings.Contains(id, "Q4_K_M") {
			record["quantization"] = "Q4_K_M"
		} else if strings.Contains(modelPath, "Q8_0") || strings.Contains(id, "Q8_0") {
			record["quantization"] = "Q8_0"
		} else if strings.Contains(modelPath, "Q5_K_M") || strings.Contains(id, "Q5_K_M") {
			record["quantization"] = "Q5_K_M"
		} else if strings.Contains(modelPath, "f16") || strings.Contains(id, "f16") {
			record["quantization"] = "F16"
		}

		// Check if model is currently loaded
		pm.Lock()
		_, isLoaded := pm.processGroups[id] // Check if process group exists for this model
		pm.Unlock()

		if isLoaded {
			record["status"] = "loaded"
			record["loading"] = false
		} else {
			record["status"] = "unloaded"
			record["loading"] = false
		}

		data = append(data, record)
	}

	// Sort by the "id" key
	sort.Slice(data, func(i, j int) bool {
		si, _ := data[i]["id"].(string)
		sj, _ := data[j]["id"].(string)
		return si < sj
	})

	// Set CORS headers if origin exists
	if origin := c.GetHeader("Origin"); origin != "" {
		c.Header("Access-Control-Allow-Origin", origin)
	}

	// Use gin's JSON method which handles content-type and encoding
	c.JSON(http.StatusOK, gin.H{
		"object": "list",
		"data":   data,
	})
}

func (pm *ProxyManager) proxyToUpstream(c *gin.Context) {
	upstreamPath := c.Param("upstreamPath")

	// If API key is required, enforce it
	if settings, _ := pm.loadSystemSettings(); settings != nil && settings.RequireAPIKey {
		key := c.GetHeader("Authorization")
		// Accept either Bearer <key> or raw key in X-API-Key
		if key == "" {
			key = c.GetHeader("X-API-Key")
		}
		if key == "" {
			// Allow API key via query param for EventSource and limited clients
			key = c.Query("api_key")
		}
		if strings.HasPrefix(strings.ToLower(key), "bearer ") {
			key = strings.TrimSpace(key[7:])
		}
		if strings.TrimSpace(key) == "" || strings.TrimSpace(settings.APIKey) == "" || key != settings.APIKey {
			pm.sendErrorResponse(c, http.StatusUnauthorized, "API key required or invalid")
			return
		}
	}

	// split the upstream path by / and search for the model name
	parts := strings.Split(strings.TrimSpace(upstreamPath), "/")
	if len(parts) == 0 {
		pm.sendErrorResponse(c, http.StatusBadRequest, "model id required in path")
		return
	}

	modelFound := false
	searchModelName := ""
	var modelName, remainingPath string
	for i, part := range parts {
		if parts[i] == "" {
			continue
		}

		if searchModelName == "" {
			searchModelName = part
		} else {
			searchModelName = searchModelName + "/" + parts[i]
		}

		if real, ok := pm.config.RealModelName(searchModelName); ok {
			modelName = real
			remainingPath = "/" + strings.Join(parts[i+1:], "/")
			modelFound = true
			break
		}
	}

	if !modelFound {
		pm.sendErrorResponse(c, http.StatusBadRequest, "model id required in path")
		return
	}

	processGroup, realModelName, err := pm.swapProcessGroup(modelName)
	if err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error swapping process group: %s", err.Error()))
		return
	}

	// rewrite the path
	c.Request.URL.Path = remainingPath
	processGroup.ProxyRequest(realModelName, c.Writer, c.Request)
}
func (pm *ProxyManager) proxyOAIHandler(c *gin.Context) {
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		pm.sendErrorResponse(c, http.StatusBadRequest, "could not ready request body")
		return
	}

	requestedModel := gjson.GetBytes(bodyBytes, "model").String()
	if requestedModel == "" {
		pm.sendErrorResponse(c, http.StatusBadRequest, "missing or invalid 'model' key")
		return
	}

	realModelName, found := pm.config.RealModelName(requestedModel)
	if !found {
		// Check if this might be a HuggingFace model that we can download
		// Support both formats: "repo/model" and "repo:filename"
		var modelToDownload string

		if strings.Contains(requestedModel, ":") {
			// Format: "repo:filename" (from our search API)
			parts := strings.SplitN(requestedModel, ":", 2)
			if len(parts) == 2 && strings.Contains(parts[0], "/") {
				// This is a valid repo:filename format
				modelToDownload = requestedModel
				pm.proxyLogger.Infof("Model %s not found locally (repo:filename format), attempting auto-download...", requestedModel)
			}
		} else if strings.Contains(requestedModel, "/") {
			// Format: "repo/model" (traditional HuggingFace format)
			modelToDownload = requestedModel
			pm.proxyLogger.Infof("Model %s not found locally (repo format), attempting auto-download...", requestedModel)
		}

		if modelToDownload != "" {
			// Trigger download and wait for it to complete
			if err := pm.autoDownloadModel(c, modelToDownload); err != nil {
				pm.sendErrorResponse(c, http.StatusServiceUnavailable, fmt.Sprintf("Failed to download model %s: %s", modelToDownload, err.Error()))
				return
			}

			// Reload configuration to pick up the new model
			// Defer save to after request completes
			saveFunc, err := pm.reloadConfigForNewModel(modelToDownload, true)
			if err != nil {
				pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("Failed to reload config after downloading %s: %s", modelToDownload, err.Error()))
				return
			}
			// Schedule config save for after request completes
			if saveFunc != nil {
				defer saveFunc()
			}

			// Try finding the model again
			realModelName, found = pm.config.RealModelName(requestedModel)
			if !found {
				// If still not found after reload, try once more with the downloaded model ID
				realModelName, found = pm.config.RealModelName(modelToDownload)
				if !found {
					// Debug: log what's in the config
					pm.proxyLogger.Errorf("Model lookup failed. Requested: %s, Downloaded: %s", requestedModel, modelToDownload)
					pm.proxyLogger.Errorf("Available models in config:")
					for modelID := range pm.config.Models {
						pm.proxyLogger.Errorf("  - %s (aliases: %v)", modelID, pm.config.Models[modelID].Aliases)
					}
					pm.proxyLogger.Errorf("Aliases map contains:")
					for alias, model := range pm.config.Aliases {
						pm.proxyLogger.Errorf("  - %s -> %s", alias, model)
					}
					pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("Model %s downloaded but still not found in config", requestedModel))
					return
				}
			}
			pm.proxyLogger.Infof("Model %s resolved to real name: %s", requestedModel, realModelName)
		} else {
			pm.sendErrorResponse(c, http.StatusBadRequest, fmt.Sprintf("could not find real modelID for %s", requestedModel))
			return
		}
	}

	processGroup, usedModelName, err := pm.swapProcessGroup(requestedModel)
	if err != nil {
		// If the swap fails, it might be because we need to use the real name
		pm.proxyLogger.Warnf("Swap failed with requested model %s, trying with real name %s", requestedModel, realModelName)
		processGroup, usedModelName, err = pm.swapProcessGroup(realModelName)
		if err != nil {
			pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error swapping process group: %s", err.Error()))
			return
		}
	}

	// Use the model name that was actually found in the process group
	modelNameForProxy := usedModelName
	pm.proxyLogger.Debugf("Using model name for proxy: %s", modelNameForProxy)

	// Track model usage for LRU eviction
	modelTracker.UpdateModelUsage(realModelName)

	// issue #69 allow custom model names to be sent to upstream
	useModelName := pm.config.Models[realModelName].UseModelName
	if useModelName != "" {
		bodyBytes, err = sjson.SetBytes(bodyBytes, "model", useModelName)
		if err != nil {
			pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error rewriting model name in JSON: %s", err.Error()))
			return
		}
	}

	// issue #174 strip parameters from the JSON body
	stripParams, err := pm.config.Models[realModelName].Filters.SanitizedStripParams()
	if err != nil { // just log it and continue
		pm.proxyLogger.Errorf("Error sanitizing strip params string: %s, %s", pm.config.Models[realModelName].Filters.StripParams, err.Error())
	} else {
		for _, param := range stripParams {
			pm.proxyLogger.Debugf("<%s> stripping param: %s", realModelName, param)
			bodyBytes, err = sjson.DeleteBytes(bodyBytes, param)
			if err != nil {
				pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error deleting parameter %s from request", param))
				return
			}
		}
	}

	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	// dechunk it as we already have all the body bytes see issue #11
	c.Request.Header.Del("transfer-encoding")
	c.Request.Header.Set("content-length", strconv.Itoa(len(bodyBytes)))
	c.Request.ContentLength = int64(len(bodyBytes))

	if err := processGroup.ProxyRequest(modelNameForProxy, c.Writer, c.Request); err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error proxying request: %s", err.Error()))
		pm.proxyLogger.Errorf("Error Proxying Request for processGroup %s and model %s", processGroup.id, modelNameForProxy)
		return
	}
}

func (pm *ProxyManager) proxyOAIPostFormHandler(c *gin.Context) {
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(32 << 20); err != nil { // 32MB max memory, larger files go to tmp disk
		pm.sendErrorResponse(c, http.StatusBadRequest, fmt.Sprintf("error parsing multipart form: %s", err.Error()))
		return
	}

	// Get model parameter from the form
	requestedModel := c.Request.FormValue("model")
	if requestedModel == "" {
		pm.sendErrorResponse(c, http.StatusBadRequest, "missing or invalid 'model' parameter in form data")
		return
	}

	processGroup, realModelName, err := pm.swapProcessGroup(requestedModel)
	if err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error swapping process group: %s", err.Error()))
		return
	}

	// We need to reconstruct the multipart form in any case since the body is consumed
	// Create a new buffer for the reconstructed request
	var requestBuffer bytes.Buffer
	multipartWriter := multipart.NewWriter(&requestBuffer)

	// Copy all form values
	for key, values := range c.Request.MultipartForm.Value {
		for _, value := range values {
			fieldValue := value
			// If this is the model field and we have a profile, use just the model name
			if key == "model" {
				// # issue #69 allow custom model names to be sent to upstream
				useModelName := pm.config.Models[realModelName].UseModelName

				if useModelName != "" {
					fieldValue = useModelName
				} else {
					fieldValue = requestedModel
				}
			}
			field, err := multipartWriter.CreateFormField(key)
			if err != nil {
				pm.sendErrorResponse(c, http.StatusInternalServerError, "error recreating form field")
				return
			}
			if _, err = field.Write([]byte(fieldValue)); err != nil {
				pm.sendErrorResponse(c, http.StatusInternalServerError, "error writing form field")
				return
			}
		}
	}

	// Copy all files from the original request
	for key, fileHeaders := range c.Request.MultipartForm.File {
		for _, fileHeader := range fileHeaders {
			formFile, err := multipartWriter.CreateFormFile(key, fileHeader.Filename)
			if err != nil {
				pm.sendErrorResponse(c, http.StatusInternalServerError, "error recreating form file")
				return
			}

			file, err := fileHeader.Open()
			if err != nil {
				pm.sendErrorResponse(c, http.StatusInternalServerError, "error opening uploaded file")
				return
			}

			if _, err = io.Copy(formFile, file); err != nil {
				file.Close()
				pm.sendErrorResponse(c, http.StatusInternalServerError, "error copying file data")
				return
			}
			file.Close()
		}
	}

	// Close the multipart writer to finalize the form
	if err := multipartWriter.Close(); err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, "error finalizing multipart form")
		return
	}

	// Create a new request with the reconstructed form data
	modifiedReq, err := http.NewRequestWithContext(
		c.Request.Context(),
		c.Request.Method,
		c.Request.URL.String(),
		&requestBuffer,
	)
	if err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, "error creating modified request")
		return
	}

	// Copy the headers from the original request
	modifiedReq.Header = c.Request.Header.Clone()
	modifiedReq.Header.Set("Content-Type", multipartWriter.FormDataContentType())

	// set the content length of the body
	modifiedReq.Header.Set("Content-Length", strconv.Itoa(requestBuffer.Len()))
	modifiedReq.ContentLength = int64(requestBuffer.Len())

	// Use the modified request for proxying
	if err := processGroup.ProxyRequest(realModelName, c.Writer, modifiedReq); err != nil {
		pm.sendErrorResponse(c, http.StatusInternalServerError, fmt.Sprintf("error proxying request: %s", err.Error()))
		pm.proxyLogger.Errorf("Error Proxying Request for processGroup %s and model %s", processGroup.id, realModelName)
		return
	}
}

func (pm *ProxyManager) sendErrorResponse(c *gin.Context, statusCode int, message string) {
	acceptHeader := c.GetHeader("Accept")

	if strings.Contains(acceptHeader, "application/json") {
		c.JSON(statusCode, gin.H{"error": message})
	} else {
		c.String(statusCode, message)
	}
}

func (pm *ProxyManager) unloadAllModelsHandler(c *gin.Context) {
	pm.StopProcesses(StopImmediately)
	c.String(http.StatusOK, "OK")
}

func (pm *ProxyManager) listRunningProcessesHandler(context *gin.Context) {
	context.Header("Content-Type", "application/json")
	runningProcesses := make([]gin.H, 0) // Default to an empty response.

	for _, processGroup := range pm.processGroups {
		for _, process := range processGroup.processes {
			if process.CurrentState() == StateReady {
				runningProcesses = append(runningProcesses, gin.H{
					"model": process.ID,
					"state": process.state,
				})
			}
		}
	}

	// Put the results under the `running` key.
	response := gin.H{
		"running": runningProcesses,
	}

	context.JSON(http.StatusOK, response) // Always return 200 OK
}

func (pm *ProxyManager) infoHandler(c *gin.Context) {
	c.Header("Content-Type", "application/json")
	modelInfo := make([]gin.H, 0)

	for modelID, modelConfig := range pm.config.Models {
		// Extract port from proxy URL if available
		port := ""
		if modelConfig.Proxy != "" {
			// Parse the proxy URL to extract port
			if strings.Contains(modelConfig.Proxy, ":") {
				parts := strings.Split(modelConfig.Proxy, ":")
				if len(parts) >= 3 {
					port = parts[len(parts)-1] // Get the last part (port)
				}
			}
		}

		// Check if model is currently running
		isRunning := false
		processGroup := pm.findGroupByModelName(modelID)
		if processGroup != nil {
			if process, exists := processGroup.processes[modelID]; exists {
				isRunning = process.CurrentState() == StateReady
			}
		}

		modelInfo = append(modelInfo, gin.H{
			"model":       modelID,
			"port":        port,
			"proxy":       modelConfig.Proxy,
			"running":     isRunning,
			"name":        modelConfig.Name,
			"description": modelConfig.Description,
		})
	}

	// Sort by model ID for consistent output
	sort.Slice(modelInfo, func(i, j int) bool {
		mi, _ := modelInfo[i]["model"].(string)
		mj, _ := modelInfo[j]["model"].(string)
		return mi < mj
	})

	response := gin.H{
		"models": modelInfo,
		"total":  len(modelInfo),
	}

	c.JSON(http.StatusOK, response)
}

func (pm *ProxyManager) findGroupByModelName(modelName string) *ProcessGroup {
	for _, group := range pm.processGroups {
		if group.HasMember(modelName) {
			return group
		}
	}
	return nil
}

// HuggingFaceFile represents a single file in a HuggingFace model
type HuggingFaceFile struct {
	Filename      string `json:"filename"`
	Size          int64  `json:"size"`
	IsSplit       bool   `json:"isSplit"`
	Quantization  string `json:"quantization"`
	SuggestedModelID string `json:"suggestedModelID"`
	DownloadURL   string `json:"downloadURL"`
}

// HuggingFaceSearchResult represents search results for a model
type HuggingFaceSearchResult struct {
	ModelID    string              `json:"modelID"`
	GGUFFiles  []HuggingFaceFile   `json:"ggufFiles"`
	TotalSize  int64               `json:"totalSize"`
}

// searchHuggingFaceModel searches for a specific model and returns GGUF file information
func (pm *ProxyManager) searchHuggingFaceModel(modelID, hfApiKey string, limit int) (*HuggingFaceSearchResult, error) {
	// Create HTTP client
	client := &http.Client{Timeout: 30 * time.Second}

	// Build HuggingFace API URL to get model details
	modelURL := fmt.Sprintf("https://huggingface.co/api/models/%s", modelID)

	// Create request
	req, err := http.NewRequest("GET", modelURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create model request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if hfApiKey != "" {
		req.Header.Set("Authorization", "Bearer "+hfApiKey)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch model info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HuggingFace API error: status %d", resp.StatusCode)
	}

	// Parse response
	var modelInfo map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&modelInfo); err != nil {
		return nil, fmt.Errorf("failed to parse model info: %v", err)
	}

	// Process siblings to find GGUF files
	result := &HuggingFaceSearchResult{
		ModelID:   modelID,
		GGUFFiles: make([]HuggingFaceFile, 0),
		TotalSize: 0,
	}

	if siblings, ok := modelInfo["siblings"].([]interface{}); ok {
		for _, sibling := range siblings {
			if siblingMap, ok := sibling.(map[string]interface{}); ok {
				if filename, ok := siblingMap["rfilename"].(string); ok && strings.HasSuffix(strings.ToLower(filename), ".gguf") {
					size := int64(0)
					if sizeVal, ok := siblingMap["size"].(float64); ok {
						size = int64(sizeVal)
					}

					// Check if this is a split model
					isSplit := false
					splitPatterns := []string{
						`-\d{5}-of-\d{5}\.gguf$`,
						`\.gguf\.part\d+of\d+$`,
						`-part-\d{5}-of-\d{5}\.gguf$`,
						`\.split-\d{5}-of-\d{5}\.gguf$`,
					}

					for _, pattern := range splitPatterns {
						if matched, _ := regexp.MatchString(pattern, filename); matched {
							isSplit = true
							break
						}
					}

					// Extract quantization from filename
					quantization := "unknown"
					if matched, _ := regexp.MatchString(`[Qq]\d+_[KkMmSsLlXxSs]`, filename); matched {
						re := regexp.MustCompile(`[Qq]\d+_[KkMmSsLlXxSs]`)
						if match := re.FindString(filename); match != "" {
							quantization = strings.ToLower(match)
						}
					} else if matched, _ := regexp.MatchString(`[Qq]\d+`, filename); matched {
						re := regexp.MustCompile(`[Qq]\d+`)
						if match := re.FindString(filename); match != "" {
							quantization = strings.ToLower(match)
						}
					} else if matched, _ := regexp.MatchString(`IQ\d+_[KkMmSsLlXxSs]+`, filename); matched {
						re := regexp.MustCompile(`IQ\d+_[KkMmSsLlXxSs]+`)
						if match := re.FindString(filename); match != "" {
							quantization = strings.ToLower(match)
						}
					}

					// Generate suggested model ID for this quantization
					suggestedModelID := modelID
					if quantization != "unknown" {
						suggestedModelID = modelID + ":" + quantization
					}

					// Create download URL
					downloadURL := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", modelID, filename)

					file := HuggingFaceFile{
						Filename:         filename,
						Size:            size,
						IsSplit:         isSplit,
						Quantization:    quantization,
						SuggestedModelID: suggestedModelID,
						DownloadURL:     downloadURL,
					}

					result.GGUFFiles = append(result.GGUFFiles, file)
					result.TotalSize += size
				}
			}
		}
	}

	return result, nil
}

// autoDownloadModel attempts to download a model from HuggingFace
func (pm *ProxyManager) autoDownloadModel(c *gin.Context, modelID string) error {
	// Extract HF API key from request headers if available
	hfApiKey := c.GetHeader("HF-Token")
	if hfApiKey == "" {
		hfApiKey = c.GetHeader("X-HF-Token")
	}

	// Parse model ID to check for specific file or quantization
	// Format can be:
	// 1. "repo/model" - download first available GGUF file
	// 2. "repo/model:filename.gguf" - download specific file (from search API)
	// 3. "repo/model:q4_k_m" - download first file matching quantization
	baseModelID := modelID
	targetFile := ""
	targetQuantization := ""

	if strings.Contains(modelID, ":") {
		parts := strings.Split(modelID, ":")
		if len(parts) == 2 {
			baseModelID = parts[0]
			// Check if it's a filename or quantization
			if strings.HasSuffix(strings.ToLower(parts[1]), ".gguf") {
				targetFile = parts[1]
				pm.proxyLogger.Infof("Auto-downloading specific file: %s from repo: %s", targetFile, baseModelID)
			} else {
				targetQuantization = strings.ToLower(parts[1])
				pm.proxyLogger.Infof("Auto-downloading first file with quantization: %s from repo: %s", targetQuantization, baseModelID)
			}
		}
	} else {
		pm.proxyLogger.Infof("Auto-downloading first available GGUF file from repo: %s", baseModelID)
	}

	// Use the enhanced search API to find available GGUF files
	searchResults, err := pm.searchHuggingFaceModel(baseModelID, hfApiKey, 50)
	if err != nil {
		pm.proxyLogger.Errorf("Failed to search for model %s: %v", baseModelID, err)
		// Fallback to old method
		return pm.autoDownloadModelFallback(c, baseModelID, hfApiKey)
	}

	if len(searchResults.GGUFFiles) == 0 {
		pm.proxyLogger.Warnf("No GGUF files found via API for model %s, trying fallback method", baseModelID)
		return pm.autoDownloadModelFallback(c, baseModelID, hfApiKey)
	}

	pm.proxyLogger.Infof("Found %d GGUF files for model %s", len(searchResults.GGUFFiles), baseModelID)

	// Use the configured download directory
	baseDownloadDir := pm.config.DownloadDir
	if baseDownloadDir == "" {
		baseDownloadDir = "./downloads"
	}
	downloadDir := filepath.Join(baseDownloadDir, strings.ReplaceAll(baseModelID, "/", "_"))

	// If a specific file is requested, download only that file
	if targetFile != "" {
		return pm.downloadSpecificFile(searchResults, targetFile, hfApiKey, downloadDir, baseModelID)
	}

	// If a specific quantization is requested, find and download only those files
	if targetQuantization != "" {
		return pm.downloadSpecificQuantization(searchResults, targetQuantization, hfApiKey, downloadDir, baseModelID)
	}

	// Otherwise, download all available GGUF files (like the UI does)
	return pm.downloadAllGGUFFiles(searchResults, hfApiKey, downloadDir, baseModelID)
}

// downloadSpecificFile downloads a specific file by filename
func (pm *ProxyManager) downloadSpecificFile(searchResults *HuggingFaceSearchResult, targetFile, hfApiKey, downloadDir, baseModelID string) error {
	// Check if file already exists
	targetPath := filepath.Join(downloadDir, targetFile)
	if _, err := os.Stat(targetPath); err == nil {
		pm.proxyLogger.Infof("File %s already exists at %s, skipping download", targetFile, targetPath)
		return nil
	}

	// Find the specific file
	var fileToDownload *HuggingFaceFile
	for _, file := range searchResults.GGUFFiles {
		if file.Filename == targetFile {
			fileToDownload = &file
			break
		}
	}

	if fileToDownload == nil {
		return fmt.Errorf("file %s not found in model %s", targetFile, baseModelID)
	}

	// Create download directory
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	// Download the specific file
	url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", baseModelID, fileToDownload.Filename)
	downloadID, err := pm.downloadManager.StartDownload(baseModelID, fileToDownload.Filename, url, hfApiKey, downloadDir)
	if err != nil {
		return fmt.Errorf("failed to start download for %s: %v", fileToDownload.Filename, err)
	}

	// Wait for download to complete
	if err := pm.waitForDownload(downloadID, 30*time.Minute); err != nil {
		return fmt.Errorf("failed to download %s: %v", fileToDownload.Filename, err)
	}

	pm.proxyLogger.Infof("Successfully downloaded file %s from %s to %s", targetFile, baseModelID, targetPath)
	return nil
}

// downloadSpecificQuantization downloads files matching a specific quantization
// For split models, downloads ALL parts; for non-split models, downloads only the first match
func (pm *ProxyManager) downloadSpecificQuantization(searchResults *HuggingFaceSearchResult, targetQuantization, hfApiKey, downloadDir, baseModelID string) error {
	// Normalize the target quantization for better matching
	// Convert q5_k -> Q5_K, q4_k_m -> Q4_K_M, etc.
	normalizedTarget := strings.ToUpper(strings.ReplaceAll(targetQuantization, "_", "_"))

	// Also try with common suffixes if not present
	alternativeTargets := []string{
		normalizedTarget,
		normalizedTarget + "_M",  // Try with _M suffix (medium)
		normalizedTarget + "_S",  // Try with _S suffix (small)
		normalizedTarget + "_L",  // Try with _L suffix (large)
		strings.TrimSuffix(normalizedTarget, "_M"), // Try without _M if present
		strings.TrimSuffix(normalizedTarget, "_S"), // Try without _S if present
		strings.TrimSuffix(normalizedTarget, "_L"), // Try without _L if present
	}

	// Log available quantizations for debugging
	pm.proxyLogger.Debugf("Looking for quantization %s (normalized: %s) in model %s", targetQuantization, normalizedTarget, baseModelID)
	pm.proxyLogger.Debugf("Available files: %v", func() []string {
		files := make([]string, 0, len(searchResults.GGUFFiles))
		for _, f := range searchResults.GGUFFiles {
			files = append(files, fmt.Sprintf("%s (%s, split:%v)", f.Filename, f.Quantization, f.IsSplit))
		}
		return files
	}())

	// Find ALL files matching the target quantization (for split models) or just first (for non-split)
	var filesToDownload []HuggingFaceFile
	var baseModelName string
	isSplitModel := false

	for _, file := range searchResults.GGUFFiles {
		fileQuant := strings.ToUpper(file.Quantization)
		for _, target := range alternativeTargets {
			if fileQuant == target || strings.Contains(fileQuant, target) {
				pm.proxyLogger.Infof("Found matching file for quantization %s: %s (quant: %s, split: %v)", target, file.Filename, file.Quantization, file.IsSplit)

				// If this is a split model, we need to download ALL parts
				if file.IsSplit {
					isSplitModel = true
					// Extract base name without split part number
					// e.g., "model-q4_k_m-00001-of-00005.gguf" -> "model-q4_k_m"
					re := regexp.MustCompile(`(.*?)(-\d{5}-of-\d{5}|\.gguf\.part\d+of\d+|-part-\d{5}-of-\d{5}|\.split-\d{5}-of-\d{5})`)
					if matches := re.FindStringSubmatch(file.Filename); len(matches) > 1 {
						baseModelName = matches[1]
					}
				}

				filesToDownload = append(filesToDownload, file)

				// If it's not a split model, we only need the first match
				if !file.IsSplit && len(filesToDownload) == 1 {
					break
				}
			}
		}
		// For non-split models, stop after finding the first match
		if !isSplitModel && len(filesToDownload) > 0 {
			break
		}
	}

	// If it's a split model and we found the base, get ALL parts with same base name
	if isSplitModel && baseModelName != "" {
		// Find all other parts of the same split model
		for _, file := range searchResults.GGUFFiles {
			if file.IsSplit && strings.HasPrefix(file.Filename, baseModelName) {
				// Check if we already have this file
				alreadyAdded := false
				for _, existing := range filesToDownload {
					if existing.Filename == file.Filename {
						alreadyAdded = true
						break
					}
				}
				if !alreadyAdded {
					pm.proxyLogger.Infof("Adding split part: %s", file.Filename)
					filesToDownload = append(filesToDownload, file)
				}
			}
		}
	}

	// If no exact match, try fuzzy match
	if len(filesToDownload) == 0 {
		pm.proxyLogger.Warnf("No exact match for quantization %s, trying fuzzy match", targetQuantization)
		basePattern := strings.Split(normalizedTarget, "_")[0]
		for _, file := range searchResults.GGUFFiles {
			if strings.Contains(strings.ToUpper(file.Quantization), basePattern) {
				pm.proxyLogger.Infof("Fuzzy match found: %s (quant: %s, split: %v) for pattern %s", file.Filename, file.Quantization, file.IsSplit, basePattern)

				// If it's a split model, get all parts
				if file.IsSplit {
					isSplitModel = true
					re := regexp.MustCompile(`(.*?)(-\d{5}-of-\d{5}|\.gguf\.part\d+of\d+|-part-\d{5}-of-\d{5}|\.split-\d{5}-of-\d{5})`)
					if matches := re.FindStringSubmatch(file.Filename); len(matches) > 1 {
						baseModelName = matches[1]
						// Get all parts with the same base name
						for _, f := range searchResults.GGUFFiles {
							if f.IsSplit && strings.HasPrefix(f.Filename, baseModelName) {
								filesToDownload = append(filesToDownload, f)
							}
						}
						break
					}
				} else {
					filesToDownload = append(filesToDownload, file)
					break // For non-split, one is enough
				}
			}
		}
	}

	if len(filesToDownload) == 0 {
		return fmt.Errorf("no file found for quantization %s in model %s (tried: %v)", targetQuantization, baseModelID, alternativeTargets)
	}

	// Create download directory
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	// Download the file(s)
	if isSplitModel {
		pm.proxyLogger.Infof("Downloading %d split parts for quantization %s", len(filesToDownload), targetQuantization)

		// Start all downloads
		var downloadIDs []string
		for _, file := range filesToDownload {
			// Check if file already exists
			targetPath := filepath.Join(downloadDir, file.Filename)
			if _, err := os.Stat(targetPath); err == nil {
				pm.proxyLogger.Infof("Split part %s already exists, skipping", file.Filename)
				continue
			}

			downloadURL := file.DownloadURL
			if downloadURL == "" {
				downloadURL = fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", baseModelID, file.Filename)
			}

			pm.proxyLogger.Infof("Downloading split part: %s (%.2f GB)", file.Filename, float64(file.Size)/(1024*1024*1024))
			downloadID, err := pm.downloadManager.StartDownload(baseModelID, file.Filename, downloadURL, hfApiKey, downloadDir)
			if err != nil {
				return fmt.Errorf("failed to start download for %s: %v", file.Filename, err)
			}
			downloadIDs = append(downloadIDs, downloadID)
		}

		// Wait for all downloads to complete
		if len(downloadIDs) > 0 {
			if err := pm.waitForMultipleDownloads(downloadIDs, 60*time.Minute); err != nil {
				return fmt.Errorf("failed to download split model parts: %v", err)
			}
		}

		pm.proxyLogger.Infof("Successfully downloaded all %d split parts", len(filesToDownload))
	} else {
		// Download single file
		file := filesToDownload[0]

		// Check if file already exists
		targetPath := filepath.Join(downloadDir, file.Filename)
		if _, err := os.Stat(targetPath); err == nil {
			pm.proxyLogger.Infof("File %s already exists at %s, skipping download", file.Filename, targetPath)
			return nil
		}

		downloadURL := file.DownloadURL
		if downloadURL == "" {
			downloadURL = fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", baseModelID, file.Filename)
		}

		pm.proxyLogger.Infof("Downloading single file: %s (%.2f GB)", file.Filename, float64(file.Size)/(1024*1024*1024))
		downloadID, err := pm.downloadManager.StartDownload(baseModelID, file.Filename, downloadURL, hfApiKey, downloadDir)
		if err != nil {
			return fmt.Errorf("failed to start download for %s: %v", file.Filename, err)
		}

		// Wait for download to complete
		if err := pm.waitForDownload(downloadID, 30*time.Minute); err != nil {
			return fmt.Errorf("failed to download %s: %v", file.Filename, err)
		}

		pm.proxyLogger.Infof("Successfully downloaded %s to %s", file.Filename, targetPath)
	}

	return nil
}

// downloadAllGGUFFiles downloads GGUF files when no specific file or quantization is specified
// For split models, downloads ALL parts; for non-split models, downloads only the first file
func (pm *ProxyManager) downloadAllGGUFFiles(searchResults *HuggingFaceSearchResult, hfApiKey, downloadDir, baseModelID string) error {
	if len(searchResults.GGUFFiles) == 0 {
		return fmt.Errorf("no GGUF files found for model %s", baseModelID)
	}

	// Check if the first file is a split model
	firstFile := searchResults.GGUFFiles[0]

	if firstFile.IsSplit {
		// This is a split model, we need to download ALL parts with the same base name
		var baseModelName string
		var filesToDownload []HuggingFaceFile

		// Extract base name without split part number
		re := regexp.MustCompile(`(.*?)(-\d{5}-of-\d{5}|\.gguf\.part\d+of\d+|-part-\d{5}-of-\d{5}|\.split-\d{5}-of-\d{5})`)
		if matches := re.FindStringSubmatch(firstFile.Filename); len(matches) > 1 {
			baseModelName = matches[1]
		}

		// Find all parts with the same base name
		for _, file := range searchResults.GGUFFiles {
			if file.IsSplit && strings.HasPrefix(file.Filename, baseModelName) {
				filesToDownload = append(filesToDownload, file)
			}
		}

		pm.proxyLogger.Infof("Downloading %d split parts for model %s", len(filesToDownload), baseModelID)

		// Create download directory
		if err := os.MkdirAll(downloadDir, 0755); err != nil {
			return fmt.Errorf("failed to create download directory: %v", err)
		}

		// Start all downloads
		var downloadIDs []string
		for _, file := range filesToDownload {
			// Check if file already exists
			targetPath := filepath.Join(downloadDir, file.Filename)
			if _, err := os.Stat(targetPath); err == nil {
				pm.proxyLogger.Infof("Split part %s already exists, skipping", file.Filename)
				continue
			}

			downloadURL := file.DownloadURL
			if downloadURL == "" {
				downloadURL = fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", baseModelID, file.Filename)
			}

			pm.proxyLogger.Infof("Downloading split part: %s (%.2f GB)", file.Filename, float64(file.Size)/(1024*1024*1024))
			downloadID, err := pm.downloadManager.StartDownload(baseModelID, file.Filename, downloadURL, hfApiKey, downloadDir)
			if err != nil {
				return fmt.Errorf("failed to start download for %s: %v", file.Filename, err)
			}
			downloadIDs = append(downloadIDs, downloadID)
		}

		// Wait for all downloads to complete
		if len(downloadIDs) > 0 {
			if err := pm.waitForMultipleDownloads(downloadIDs, 60*time.Minute); err != nil {
				return fmt.Errorf("failed to download split model parts: %v", err)
			}
		}

		pm.proxyLogger.Infof("Successfully downloaded all %d split parts", len(filesToDownload))
		return nil
	}

	// Not a split model, download only the first file
	// Check if file already exists
	targetPath := filepath.Join(downloadDir, firstFile.Filename)
	if _, err := os.Stat(targetPath); err == nil {
		pm.proxyLogger.Infof("File %s already exists at %s, skipping download", firstFile.Filename, targetPath)
		return nil
	}

	// Create download directory
	if err := os.MkdirAll(downloadDir, 0755); err != nil {
		return fmt.Errorf("failed to create download directory: %v", err)
	}

	pm.proxyLogger.Infof("Downloading first available GGUF file: %s (%.2f GB)", firstFile.Filename, float64(firstFile.Size)/(1024*1024*1024))

	// Build download URL if not provided
	downloadURL := firstFile.DownloadURL
	if downloadURL == "" {
		downloadURL = fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", baseModelID, firstFile.Filename)
	}

	// Download the single file
	downloadID, err := pm.downloadManager.StartDownload(baseModelID, firstFile.Filename, downloadURL, hfApiKey, downloadDir)
	if err != nil {
		return fmt.Errorf("failed to start download for %s: %v", firstFile.Filename, err)
	}

	// Wait for download to complete
	if err := pm.waitForDownload(downloadID, 30*time.Minute); err != nil {
		return fmt.Errorf("failed to download %s: %v", firstFile.Filename, err)
	}

	pm.proxyLogger.Infof("Successfully downloaded %s to %s", firstFile.Filename, targetPath)
	return nil
}

// waitForMultipleDownloads waits for multiple downloads to complete
func (pm *ProxyManager) waitForMultipleDownloads(downloadIDs []string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	completedCount := 0
	totalCount := len(downloadIDs)

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("downloads timed out after %v (%d/%d completed)", timeout, completedCount, totalCount)
		case <-ticker.C:
			newCompletedCount := 0
			var failedDownloads []string

			for _, downloadID := range downloadIDs {
				status := pm.downloadManager.GetDownloadStatus(downloadID)
				if status == nil {
					failedDownloads = append(failedDownloads, downloadID)
					continue
				}

				switch status.Status {
				case StatusCompleted:
					newCompletedCount++
				case StatusFailed, StatusCancelled:
					failedDownloads = append(failedDownloads, fmt.Sprintf("%s (%s)", downloadID, status.Error))
				}
			}

			completedCount = newCompletedCount

			// Check if there are any failures
			if len(failedDownloads) > 0 {
				return fmt.Errorf("some downloads failed: %v", failedDownloads)
			}

			// Check if all downloads are complete
			if completedCount == totalCount {
				pm.proxyLogger.Infof("All %d downloads completed successfully", totalCount)
				return nil
			}

			// Log progress every few iterations
			if completedCount > 0 {
				pm.proxyLogger.Infof("Download progress: %d/%d files completed", completedCount, totalCount)
			}
		}
	}
}

// autoDownloadModelFallback provides fallback behavior for when search API fails
func (pm *ProxyManager) autoDownloadModelFallback(c *gin.Context, modelID, hfApiKey string) error {
	// Use the original logic as fallback
	commonQuantizations := []string{
		"Q4_K_M", "Q5_K_M", "Q6_K", "Q8_0",
	}

	// Use the configured download directory
	baseDownloadDir := pm.config.DownloadDir
	if baseDownloadDir == "" {
		baseDownloadDir = "./downloads"
	}
	downloadDir := filepath.Join(baseDownloadDir, strings.ReplaceAll(modelID, "/", "_"))

	// Try to download the first available quantization
	for _, quant := range commonQuantizations {
		filename := fmt.Sprintf("%s.gguf", quant)
		url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s/%s", modelID, quant, filename)

		downloadID, err := pm.downloadManager.StartDownload(modelID, filename, url, hfApiKey, downloadDir)
		if err == nil {
			// Wait for download to complete (with timeout)
			return pm.waitForDownload(downloadID, 30*time.Minute)
		}
	}

	return fmt.Errorf("no suitable GGUF files found for model %s", modelID)
}

// waitForDownload waits for a download to complete or timeout
func (pm *ProxyManager) waitForDownload(downloadID string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("download timed out after %v", timeout)
		case <-ticker.C:
			status := pm.downloadManager.GetDownloadStatus(downloadID)
			if status == nil {
				return fmt.Errorf("download %s not found", downloadID)
			}

			switch status.Status {
			case StatusCompleted:
				return nil
			case StatusFailed, StatusCancelled:
				return fmt.Errorf("download failed: %s", status.Error)
			}
		}
	}
}

// reloadConfigForNewModel adds a newly downloaded model to the configuration
// It updates the in-memory config immediately and optionally schedules a file write
// If deferSave is true, it returns a function to save the config that should be called after the request completes
func (pm *ProxyManager) reloadConfigForNewModel(modelID string, deferSave bool) (saveFunc func(), err error) {
	// After downloading a model, we need to add it to the config
	configPath := pm.configPath

	// First check if this model ID already exists in config
	if _, found := pm.config.RealModelName(modelID); found {
		pm.proxyLogger.Infof("Model %s already exists in config, skipping config update", modelID)
		return nil, nil
	}

	// Parse the model ID to get the actual file path
	baseModelID := modelID
	targetFile := ""
	targetQuantization := ""

	if strings.Contains(modelID, ":") {
		parts := strings.Split(modelID, ":")
		baseModelID = parts[0]
		if len(parts) > 1 {
			if strings.HasSuffix(strings.ToLower(parts[1]), ".gguf") {
				targetFile = parts[1]
			} else {
				// It's a quantization like "q5_k"
				targetQuantization = strings.ToLower(parts[1])
			}
		}
	}

	// Generate a model ID for the config
	configModelID := strings.ReplaceAll(baseModelID, "/", "-")
	if targetFile != "" {
		// Remove .gguf extension for the ID
		configModelID = configModelID + "-" + strings.TrimSuffix(targetFile, ".gguf")
	} else if targetQuantization != "" {
		// Use quantization in the ID
		configModelID = configModelID + "-" + targetQuantization
	}
	configModelID = strings.ToLower(configModelID)

	// Check if this config model ID already exists
	if _, exists := pm.config.Models[configModelID]; exists {
		pm.proxyLogger.Infof("Model config entry %s already exists, checking if aliases need updating", configModelID)
		// Model already exists, just reload config to ensure it's up to date
		newConfig, err := LoadConfig(configPath)
		if err != nil {
			return nil, fmt.Errorf("failed to reload config: %v", err)
		}
		pm.Lock()
		pm.config = newConfig
		pm.Unlock()
		return nil, nil
	}

	// Variables to hold model config data for later
	var modelConfigToSave map[string]interface{}
	var absModelPath string

	// Build the model path using configured download directory
	baseDownloadDir := pm.config.DownloadDir
	if baseDownloadDir == "" {
		baseDownloadDir = "./downloads"
	}
	downloadDir := filepath.Join(baseDownloadDir, strings.ReplaceAll(baseModelID, "/", "_"))

	// If we don't have a specific file, find what was downloaded
	if targetFile == "" {
		// Find the first GGUF file in the directory (including subdirectories)
		// First try root directory
		files, err := filepath.Glob(filepath.Join(downloadDir, "*.gguf"))
		if err != nil || len(files) == 0 {
			// Try subdirectories (common for quantization-specific downloads)
			files, err = filepath.Glob(filepath.Join(downloadDir, "*", "*.gguf"))
		}
		if err == nil && len(files) > 0 {
			modelPath := files[0]
			// Get the relative path from downloadDir for proper construction later
			relPath, _ := filepath.Rel(downloadDir, modelPath)
			targetFile = relPath
			pm.proxyLogger.Infof("Found downloaded file: %s", targetFile)
		} else {
			pm.proxyLogger.Warnf("No GGUF files found in %s or its subdirectories", downloadDir)
			return nil, fmt.Errorf("no GGUF files found in download directory")
		}
	}

	modelPath := filepath.Join(downloadDir, targetFile)

	// Check if the model file actually exists
	modelExists := true
	if _, err := os.Stat(modelPath); os.IsNotExist(err) {
		// Try absolute path
		absPath, _ := filepath.Abs(modelPath)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			pm.proxyLogger.Warnf("Downloaded model not found at %s or %s", modelPath, absPath)
			modelExists = false
		} else {
			modelPath = absPath
		}
	}

	// Get absolute path for the model (even if it doesn't exist yet)
	absModelPath, _ = filepath.Abs(modelPath)

	// Find llama-server binary path for the config file
	llamaServerPath := "llama-server" // Default to PATH lookup

	// Check common locations for llama-server binary
	possiblePaths := []string{
		"./llama-server",
		"./binaries/llama-server/llama-server",
		"./binaries/llama-server/build/bin/llama-server",
		"/usr/local/bin/llama-server",
		"/usr/bin/llama-server",
	}

	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			llamaServerPath, _ = filepath.Abs(path)
			break
		}
	}

	// Load the current config to find the next available port
	currentConfig, _ := LoadConfig(configPath)
	nextPort := currentConfig.StartPort
	if nextPort == 0 {
		nextPort = 8080 // Default if not set
	}

	// Check all existing models to find the highest port in use
	for _, existingModel := range currentConfig.Models {
		// Extract port from proxy URL if it exists
		if existingModel.Proxy != "" && !strings.Contains(existingModel.Proxy, "${PORT}") {
			// Try to extract port from proxy URL like http://127.0.0.1:10001
			if parts := strings.Split(existingModel.Proxy, ":"); len(parts) >= 3 {
				portStr := strings.TrimPrefix(parts[len(parts)-1], "//")
				if port, err := strconv.Atoi(portStr); err == nil && port >= nextPort {
					nextPort = port + 1
				}
			}
		}
	}

	// Allocate the port now so we can save it to the config file
	allocatedPort := strconv.Itoa(nextPort)

	// Create a model configuration entry with the actual allocated port
	modelConfig := map[string]interface{}{
		"name": baseModelID,
		"cmd": fmt.Sprintf(`%s --host 127.0.0.1 --port %s
  --model %s
  --ctx-size 4096
  -ngl 999`, llamaServerPath, allocatedPort, absModelPath),
		"proxy": fmt.Sprintf("http://127.0.0.1:%s", allocatedPort),
		"ttl": 300,
	}

	// ALWAYS add aliases for different formats of the model ID
	// This ensures the model can be found by its original requested ID
	if true { // Keep the same indentation for minimal changes
		// Add aliases for different formats of the model ID
		// But first, check which aliases already exist in the config
		aliases := []string{}
		aliasesToAdd := []string{}

		// Add the original requested format as an alias
		aliasesToAdd = append(aliasesToAdd, modelID)

		// Add repo/model format
		if baseModelID != modelID {
			aliasesToAdd = append(aliasesToAdd, baseModelID)
		}

		// Add repo:quantization format if we can extract quantization
		if strings.Contains(targetFile, "_") {
			// Try to extract quantization from filename
			// e.g., "Mistral-22B-v0.1-Q5_K_M.gguf" -> "Q5_K_M"
			parts := strings.Split(targetFile, "-")
			for _, part := range parts {
				if matched, _ := regexp.MatchString(`[Qq]\d+_[KkMmSsLl]`, part); matched {
					quantAlias := baseModelID + ":" + strings.ToLower(part)
					aliasesToAdd = append(aliasesToAdd, quantAlias)
					// Also add without the _M/_S/_L suffix
					baseQuant := strings.Split(strings.ToLower(part), "_")
					if len(baseQuant) >= 2 {
						aliasesToAdd = append(aliasesToAdd, baseModelID + ":" + baseQuant[0] + "_" + baseQuant[1])
					}
					break
				}
			}
			// Also check in the middle of filename for quantization patterns
			if matched, _ := regexp.MatchString(`[Qq]\d+_[KkMmSsLl]`, targetFile); matched {
				re := regexp.MustCompile(`[Qq]\d+_[KkMmSsLl][_MmSsLl]*`)
				if match := re.FindString(targetFile); match != "" {
					quantAlias := baseModelID + ":" + strings.ToLower(match)
					aliasesToAdd = append(aliasesToAdd, quantAlias)
				}
			}
		}

		// Deduplicate aliases first
		uniqueAliases := make(map[string]bool)
		for _, alias := range aliasesToAdd {
			uniqueAliases[alias] = true
		}

		// Filter out aliases that already exist in the config
		currentConfig, _ := LoadConfig(configPath)
		for alias := range uniqueAliases {
			if _, found := currentConfig.RealModelName(alias); !found {
				// This alias doesn't exist yet, safe to add
				aliases = append(aliases, alias)
			} else {
				pm.proxyLogger.Debugf("Alias %s already exists in config, skipping", alias)
			}
		}

		if len(aliases) > 0 {
			modelConfig["aliases"] = aliases
		}

		// Save modelConfig for later use
		modelConfigToSave = modelConfig
	}

	// If not deferring save, schedule the file write for after this function returns (only if model exists)
	if !deferSave && modelExists && modelConfigToSave != nil {
		defer func() {
			// Write to file in background AFTER the request is done
			go func() {
				// Wait longer to ensure the request completes and response is sent
				// This prevents the config reload from killing the model mid-request
				time.Sleep(5 * time.Second) // Increased delay to let request complete
				if err := pm.appendModelToConfig(configPath, configModelID, modelConfigToSave); err != nil {
					pm.proxyLogger.Errorf("Failed to save model to config file: %v", err)
				} else {
					pm.proxyLogger.Infof("Model %s persisted to config file", configModelID)
				}
			}()
		}()
	}

	// Load the current config (without the new model yet)
	newConfig, err := LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to reload config: %v", err)
	}

	// Manually add the new model to the in-memory config
	if newConfig.Models == nil {
		newConfig.Models = make(map[string]ModelConfig)
	}

	// Convert the modelConfig map to ModelConfig struct
	// Use a default path if absModelPath is empty
	finalModelPath := absModelPath
	if finalModelPath == "" {
		// Fallback to a reasonable default path
		finalModelPath = filepath.Join(downloadDir, targetFile)
		if targetFile == "" {
			finalModelPath = filepath.Join(downloadDir, "model.gguf")
		}
	}

	// We already have the port from modelConfigToSave, extract it
	var portStr string
	if modelConfigToSave != nil {
		// Extract port from the saved proxy URL
		if proxyVal, ok := modelConfigToSave["proxy"].(string); ok {
			// Extract port from proxy URL like http://127.0.0.1:10001
			if parts := strings.Split(proxyVal, ":"); len(parts) >= 3 {
				portStr = strings.TrimPrefix(parts[len(parts)-1], "//")
			}
		}
	}

	// If we couldn't extract the port, allocate a new one
	if portStr == "" {
		nextPort := newConfig.StartPort
		if nextPort == 0 {
			nextPort = 8080 // Default if not set
		}

		// Check all existing models to find the highest port in use
		for _, existingModel := range newConfig.Models {
			// Extract port from proxy URL if it exists
			if existingModel.Proxy != "" && !strings.Contains(existingModel.Proxy, "${PORT}") {
				// Try to extract port from proxy URL like http://127.0.0.1:10001
				if parts := strings.Split(existingModel.Proxy, ":"); len(parts) >= 3 {
					p := strings.TrimPrefix(parts[len(parts)-1], "//")
					if port, err := strconv.Atoi(p); err == nil && port >= nextPort {
						nextPort = port + 1
					}
				}
			}
		}
		portStr = strconv.Itoa(nextPort)
	}

	// Build the command - try to use llama-server-base macro if available, otherwise use direct path
	var cmdTemplate string

	if llamaServerBase, hasMacro := newConfig.Macros["llama-server-base"]; hasMacro && llamaServerBase != "" {
		// Macro exists, expand it directly with the allocated port
		expandedMacro := strings.ReplaceAll(llamaServerBase, "${PORT}", portStr)
		cmdTemplate = fmt.Sprintf("%s\n--model %s\n--ctx-size 4096\n-ngl 999", expandedMacro, finalModelPath)
		pm.proxyLogger.Infof("Using llama-server-base macro for command with port %s", portStr)
	} else {
		// No macro, try to find llama-server binary
		llamaServerPath := "llama-server" // Default to PATH lookup

		// Check common locations
		possiblePaths := []string{
			"./llama-server",
			"./binaries/llama-server/llama-server",
			"./binaries/llama-server/build/bin/llama-server",
			"/usr/local/bin/llama-server",
			"/usr/bin/llama-server",
		}

		for _, path := range possiblePaths {
			if _, err := os.Stat(path); err == nil {
				llamaServerPath = path
				break
			}
		}

		// Build command with direct binary path and allocated port
		cmdTemplate = fmt.Sprintf("%s --host 127.0.0.1 --port %s --model %s --ctx-size 4096 -ngl 999", llamaServerPath, portStr, finalModelPath)
		pm.proxyLogger.Warnf("No llama-server-base macro found, using direct binary: %s with port %s", llamaServerPath, portStr)
	}

	// Expand ${PORT} in the command template if it still exists
	cmdTemplate = strings.ReplaceAll(cmdTemplate, "${PORT}", portStr)
	proxyURL := fmt.Sprintf("http://127.0.0.1:%s", portStr)

	modelConfigStruct := ModelConfig{
		Name:        configModelID,
		Cmd:         cmdTemplate,
		Proxy:       proxyURL,
		UnloadAfter: 300,
	}

	// If we have the saved config, use some of its values but NOT the unexpanded cmd/proxy
	if modelConfigToSave != nil {
		if nameVal, ok := modelConfigToSave["name"]; ok {
			modelConfigStruct.Name = nameVal.(string)
		}
		// Don't use cmd from saved config as it has unexpanded ${PORT}
		// We already built the expanded cmd above

		// Don't use proxy from saved config as it has unexpanded ${PORT}
		// We already set the expanded proxy above

		if ttlVal, ok := modelConfigToSave["ttl"]; ok {
			modelConfigStruct.UnloadAfter = ttlVal.(int)
		}
		// Add aliases if present
		if aliasesVal, ok := modelConfigToSave["aliases"]; ok {
			// Handle both []string and []interface{} types
			switch v := aliasesVal.(type) {
			case []string:
				modelConfigStruct.Aliases = v
			case []interface{}:
				aliases := make([]string, len(v))
				for i, alias := range v {
					if str, ok := alias.(string); ok {
						aliases[i] = str
					}
				}
				modelConfigStruct.Aliases = aliases
			}
		}
	}

	// Add the model to the config
	newConfig.Models[configModelID] = modelConfigStruct
	pm.proxyLogger.Infof("Added model to config with ID: %s, aliases: %v", configModelID, modelConfigStruct.Aliases)

	// Ensure the model is in a group
	if newConfig.Groups == nil {
		newConfig.Groups = make(map[string]GroupConfig)
	}

	// Add to all-models group or create it
	if allModelsGroup, exists := newConfig.Groups["all-models"]; exists {
		// Add to existing group if not already there
		found := false
		for _, member := range allModelsGroup.Members {
			if member == configModelID {
				found = true
				break
			}
		}
		if !found {
			allModelsGroup.Members = append(allModelsGroup.Members, configModelID)
			newConfig.Groups["all-models"] = allModelsGroup
		}
	} else {
		// Create the all-models group
		newConfig.Groups["all-models"] = GroupConfig{
			Swap:       true,
			Exclusive:  false,
			Persistent: false,
			Members:    []string{configModelID},
		}
	}

	// CRITICAL: Rebuild the aliases map so RealModelName can find the model
	// The aliases map is normally built during LoadConfig, but we're manually updating
	newConfig.Aliases = make(map[string]string)
	for modelName, modelConfig := range newConfig.Models {
		for _, alias := range modelConfig.Aliases {
			newConfig.Aliases[alias] = modelName
			pm.proxyLogger.Debugf("Added alias mapping: %s -> %s", alias, modelName)
		}
	}

	// CRITICAL: Update in-memory config and process groups atomically
	// This ensures the model is available immediately for the current request
	pm.Lock()
	pm.config = newConfig

	// Reinitialize process groups for any new models
	for groupName, groupConfig := range newConfig.Groups {
		// Check if this group already exists
		if existingGroup, exists := pm.processGroups[groupName]; !exists {
			// Create new process group
			pm.processGroups[groupName] = NewProcessGroup(groupName, newConfig, pm.proxyLogger, pm.upstreamLogger)
			pm.proxyLogger.Infof("Created new process group: %s with members: %v", groupName, groupConfig.Members)
		} else {
			// Update the existing group's config reference so HasMember works correctly
			existingGroup.config = newConfig

			// Update existing group with new members
			existingGroup.Lock() // Lock the group while modifying processes
			for _, memberName := range groupConfig.Members {
				if _, hasProcess := existingGroup.processes[memberName]; !hasProcess {
					// Add the new member to the existing group
					if modelConfig, ok := newConfig.Models[memberName]; ok {
						process := NewProcess(memberName, newConfig.HealthCheckTimeout, modelConfig, pm.upstreamLogger, pm.proxyLogger)
						existingGroup.processes[memberName] = process
						pm.proxyLogger.Infof("Added process for model %s to existing group %s", memberName, groupName)
					} else {
						pm.proxyLogger.Warnf("Model config not found for %s when adding to group %s", memberName, groupName)
					}
				}
			}
			existingGroup.Unlock()
		}
	}

	// The config is now updated in THIS ProxyManager instance
	// The file watcher might trigger a reload, but that will create a NEW ProxyManager
	// This request will continue using the current one with the model already loaded
	pm.Unlock()

	// Give a tiny moment for the process group to be fully initialized
	// This helps avoid any potential race conditions
	time.Sleep(10 * time.Millisecond)

	// Emit config changed event
	event.Emit(ConfigFileChangedEvent{
		ReloadingState: ReloadingStateEnd,
	})

	pm.proxyLogger.Infof("Config reloaded with new model %s - model is now ready for use", configModelID)

	// Return save function if deferSave is true
	if deferSave && modelExists && modelConfigToSave != nil {
		return func() {
			if err := pm.appendModelToConfig(configPath, configModelID, modelConfigToSave); err != nil {
				pm.proxyLogger.Errorf("Failed to save model to config file: %v", err)
			} else {
				pm.proxyLogger.Infof("Model %s persisted to config file", configModelID)
			}
		}, nil
	}

	return nil, nil
}

// ensureMemoryAvailable checks if there's enough memory to load a model
// and unloads other models if necessary to free up memory
func (pm *ProxyManager) ensureMemoryAvailable(group *ProcessGroup, modelName string) error {
	// Get the minimum free memory percentage from config or use default
	minFreePercent := pm.config.MinFreeMemoryPercent
	if minFreePercent == 0 {
		minFreePercent = 10.0 // Default to 10% if not set
	}

	// Get current memory stats
	memInfo, err := pm.getMemoryInfo()
	if err != nil {
		pm.proxyLogger.Warnf("Could not get memory info, proceeding anyway: %v", err)
		return nil // Don't block if we can't get memory info
	}

	// Calculate required free memory
	requiredFreeBytes := uint64(float64(memInfo.Total) * (minFreePercent / 100.0))

	// If we already have enough free memory, we're good
	if memInfo.Available >= requiredFreeBytes {
		return nil
	}

	pm.proxyLogger.Infof("Memory below threshold: %.1f%% free (need %.1f%%), unloading models...",
		float64(memInfo.Available)/float64(memInfo.Total)*100, minFreePercent)

	// Unload non-persistent models to free up memory
	// Start with the least recently used models
	unloadedCount := 0
	for groupId, otherGroup := range pm.processGroups {
		if groupId != group.id && !otherGroup.persistent {
			otherGroup.StopProcesses(StopImmediately)
			unloadedCount++

			// Check memory again after unloading
			memInfo, err = pm.getMemoryInfo()
			if err == nil && memInfo.Available >= requiredFreeBytes {
				pm.proxyLogger.Infof("Unloaded %d models to free memory", unloadedCount)
				return nil
			}
		}
	}

	// If we still don't have enough memory after unloading everything possible
	if memInfo.Available < requiredFreeBytes {
		return fmt.Errorf("insufficient memory even after unloading %d models (have %.1fGB free, need %.1fGB)",
			unloadedCount,
			float64(memInfo.Available)/(1024*1024*1024),
			float64(requiredFreeBytes)/(1024*1024*1024))
	}

	return nil
}

// MemoryInfo represents system memory statistics
type MemoryInfo struct {
	Total     uint64 // Total system memory in bytes
	Available uint64 // Available memory in bytes
	Used      uint64 // Used memory in bytes
}

// getMemoryInfo retrieves current system memory statistics
func (pm *ProxyManager) getMemoryInfo() (*MemoryInfo, error) {
	// This is a simplified implementation
	// In production, this would use system-specific calls to get actual memory info
	// For Linux, we'd read from /proc/meminfo
	// For Windows, we'd use Windows API calls
	// For macOS, we'd use sysctl

	// For now, return a mock implementation
	// You would replace this with actual system calls
	info, err := autosetup.GetRealtimeHardwareInfo()
	if err != nil {
		return nil, err
	}

	return &MemoryInfo{
		Total:     uint64(info.TotalRAMGB * 1024 * 1024 * 1024),
		Available: uint64(info.AvailableRAMGB * 1024 * 1024 * 1024),
		Used:      uint64((info.TotalRAMGB - info.AvailableRAMGB) * 1024 * 1024 * 1024),
	}, nil
}

// gpuStatsHandler returns GPU statistics
func (pm *ProxyManager) gpuStatsHandler(c *gin.Context) {
	// Get GPU statistics
	gpuInfo, err := autosetup.DetectAllGPUs()
	if err != nil {
		// Return empty GPU list if no GPUs found
		c.JSON(http.StatusOK, gin.H{
			"gpus":         []interface{}{},
			"totalGPUs":    0,
			"totalMemory":  0,
			"totalFree":    0,
			"backend":      "cpu",
			"error":        err.Error(),
		})
		return
	}

	// Get memory info as well
	memInfo, _ := pm.getMemoryInfo()

	response := gin.H{
		"gpus":          gpuInfo.GPUs,
		"totalGPUs":     gpuInfo.TotalGPUs,
		"totalMemory":   gpuInfo.TotalMemory,
		"totalFree":     gpuInfo.TotalFree,
		"backend":       gpuInfo.Backend,
		"driverVersion": gpuInfo.DriverVersion,
	}

	// Add system memory info
	if memInfo != nil {
		totalGB := float64(memInfo.Total) / (1024 * 1024 * 1024)
		availableGB := float64(memInfo.Available) / (1024 * 1024 * 1024)
		usedGB := float64(memInfo.Used) / (1024 * 1024 * 1024)
		usagePercent := (usedGB / totalGB) * 100

		response["systemRAM"] = gin.H{
			"total":        totalGB,
			"free":         availableGB,
			"used":         usedGB,
			"usagePercent": usagePercent,
		}
	}

	c.JSON(http.StatusOK, response)
}
