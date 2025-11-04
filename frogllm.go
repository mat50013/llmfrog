package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/gin-gonic/gin"
	"github.com/prave/FrogLLM/autosetup"
	"github.com/prave/FrogLLM/event"
	"github.com/prave/FrogLLM/proxy"
)

var (
	version string = "0"
	commit  string = "abcd1234"
	date    string = "unknown"
)

func main() {
	// Define a command-line flag for the port
	configPath := flag.String("config", "config.yaml", "config file name")
	listenStr := flag.String("listen", ":5800", "listen ip/port for FrogLLM web interface")
	showVersion := flag.Bool("version", false, "show version of build")
	watchConfig := flag.Bool("watch-config", true, "Automatically reload config file on change (default: true)")
	modelsFolder := flag.String("models-folder", "", "automatically detect GGUF models in folder and generate config")
	autoDraft := flag.Bool("auto-draft", false, "enable automatic draft model pairing for speculative decoding")
	enableJinja := flag.Bool("jinja", true, "enable Jinja templating support for models (default: true)")
	parallel := flag.Bool("parallel", true, "enable parallel processing for faster setup (default: true)")
	realtime := flag.Bool("realtime", false, "enable real-time hardware monitoring for dynamic memory allocation (recommended for home PCs)")

	// Hardware override flags for initialization
	forceBackend := flag.String("backend", "", "force specific backend (cuda, rocm, cpu, vulkan) - overrides auto-detection")
	forceRAM := flag.Float64("ram", 0, "force total RAM in GB - overrides auto-detection (e.g. --ram 64)")
	forceVRAM := flag.Float64("vram", 0, "force total VRAM in GB - overrides auto-detection (e.g. --vram 24)")
	minFreeMemoryPercent := flag.Float64("min-free-memory", 10.0, "minimum percentage of memory to keep free (default: 10%)")
	llamaServerPath := flag.String("llama-server-path", "", "custom path to llama-server binary - overrides auto-download")
	llamaServer := flag.String("llama-server", "", "replace llama-server binary path in existing config and rebuild")
	hfToken := flag.String("hf-token", "", "Hugging Face API token for downloading private models")

	flag.Parse() // Parse the command-line flags

	if *showVersion {
		fmt.Printf("version: %s (%s), built at %s\n", version, commit, date)
		os.Exit(0)
	}

	// Handle --llama-server flag to replace binary path in config
	if *llamaServer != "" {
		if err := replaceLlamaServerInConfig(*configPath, *llamaServer); err != nil {
			fmt.Printf("Error replacing llama-server path: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("✅ Updated llama-server path to: %s\n", *llamaServer)
		fmt.Println("🔨 Rebuilding with new configuration...")

		// Run build script
		if err := runBuildScript(); err != nil {
			fmt.Printf("Error running build script: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("✅ Build completed successfully!")
		os.Exit(0)
	}

	// Ensure config file exists; create an empty one if missing
	if _, statErr := os.Stat(*configPath); statErr != nil {
		if os.IsNotExist(statErr) {
			// Create parent directory if necessary
			if err := os.MkdirAll(filepath.Dir(*configPath), 0755); err != nil {
				fmt.Printf("Error creating config directory: %v\n", err)
				os.Exit(1)
			}
			if err := os.WriteFile(*configPath, []byte{}, 0644); err != nil {
				fmt.Printf("Error creating empty config file: %v\n", err)
				os.Exit(1)
			}
			fmt.Printf("Created empty config at %s\n", *configPath)
		} else {
			fmt.Printf("Error checking config file: %v\n", statErr)
			os.Exit(1)
		}
	}

	// Handle auto-setup mode
	if *modelsFolder != "" {
		fmt.Println("Running auto-setup mode...")
		err := autosetup.AutoSetupWithOptions(*modelsFolder, autosetup.SetupOptions{
			EnableDraftModels:    *autoDraft,
			EnableJinja:          *enableJinja,
			EnableParallel:       *parallel,
			EnableRealtime:       *realtime,
			ForceBackend:         *forceBackend,
			ForceRAM:             *forceRAM,
			ForceVRAM:            *forceVRAM,
			MinFreeMemoryPercent: *minFreeMemoryPercent,
			LlamaServerPath:      *llamaServerPath,
		})
		if err != nil {
			fmt.Printf("Auto-setup failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Auto-setup completed successfully!")
		fmt.Println("🚀 Starting FrogLLM server with the generated configuration...")
		fmt.Println("📁 Config watching is enabled - any changes to config.yaml will trigger automatic reloads")
		fmt.Printf("🌐 Server will be available at: http://localhost%s\n", *listenStr)
		fmt.Printf("🎛️  Web interface: http://localhost%s/ui/\n", *listenStr)
		fmt.Println("💡 You can now edit config.yaml manually or use the web interface - changes will auto-reload!")
		// Continue to start the server instead of exiting
	}

	config, err := proxy.LoadConfig(*configPath)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		// Attempt auto-regeneration from DB to self-heal common config errors
		if selfHealReconfigure(*configPath) {
			fmt.Println("Self-heal: regenerated configuration from tracked folders. Retrying load...")
			config, err = proxy.LoadConfig(*configPath)
		}
		if err != nil {
			os.Exit(1)
		}
	}

	// Override minFreeMemoryPercent from command line if provided
	if *minFreeMemoryPercent > 0 {
		config.MinFreeMemoryPercent = *minFreeMemoryPercent
		fmt.Printf("✅ Memory threshold set to %.1f%% (overriding config)\n", *minFreeMemoryPercent)
	}

	if len(config.Profiles) > 0 {
		fmt.Println("WARNING: Profile functionality has been removed in favor of Groups. See the README for more information.")
	}

	if mode := os.Getenv("GIN_MODE"); mode != "" {
		gin.SetMode(mode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Setup channels for server management
	exitChan := make(chan struct{})
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create server with initial handler
	srv := &http.Server{
		Addr: *listenStr,
	}

	// Support for watching config and reloading when it changes
	reloadProxyManager := func() {
		if currentPM, ok := srv.Handler.(*proxy.ProxyManager); ok {
			config, err = proxy.LoadConfig(*configPath)
			if err != nil {
				fmt.Printf("Warning, unable to reload configuration: %v\n", err)
				return
			}

			// Override minFreeMemoryPercent from command line if provided
			if *minFreeMemoryPercent > 0 {
				config.MinFreeMemoryPercent = *minFreeMemoryPercent
			}

			fmt.Println("📝 Configuration file changed - reloading...")
			currentPM.Shutdown()
			pm := proxy.New(config)
			pm.SetConfigPath(*configPath)
			srv.Handler = pm
			fmt.Println("✅ Configuration reloaded successfully")

			// wait a few seconds and tell any UI to reload
			time.AfterFunc(3*time.Second, func() {
				event.Emit(proxy.ConfigFileChangedEvent{
					ReloadingState: proxy.ReloadingStateEnd,
				})
			})
		} else {
			config, err = proxy.LoadConfig(*configPath)
			if err != nil {
				fmt.Printf("Error, unable to load configuration: %v\n", err)
				if selfHealReconfigure(*configPath) {
					fmt.Println("Self-heal: regenerated configuration from tracked folders. Retrying load...")
					config, err = proxy.LoadConfig(*configPath)
				}
				if err != nil {
					os.Exit(1)
				}
			}
			pm := proxy.New(config)
			pm.SetConfigPath(*configPath)
			srv.Handler = pm
		}
	}

	// load the initial proxy manager
	reloadProxyManager()
	debouncedReload := debounce(time.Second, reloadProxyManager)
	if *watchConfig {
		defer event.On(func(e proxy.ConfigFileChangedEvent) {
			if e.ReloadingState == proxy.ReloadingStateStart {
				debouncedReload()
			}
		})()

		fmt.Printf("📁 Watching %s for changes - server will auto-reload when config changes\n", *configPath)
		go func() {
			absConfigPath, err := filepath.Abs(*configPath)
			if err != nil {
				fmt.Printf("Error getting absolute path for watching config file: %v\n", err)
				return
			}
			watcher, err := fsnotify.NewWatcher()
			if err != nil {
				fmt.Printf("Error creating file watcher: %v. File watching disabled.\n", err)
				return
			}

			configDir := filepath.Dir(absConfigPath)
			err = watcher.Add(configDir)
			if err != nil {
				fmt.Printf("Error adding config path directory (%s) to watcher: %v. File watching disabled.", configDir, err)
				return
			}

			defer watcher.Close()
			for {
				select {
				case changeEvent := <-watcher.Events:
					if changeEvent.Name == absConfigPath && (changeEvent.Has(fsnotify.Write) || changeEvent.Has(fsnotify.Create) || changeEvent.Has(fsnotify.Remove)) {
						event.Emit(proxy.ConfigFileChangedEvent{
							ReloadingState: proxy.ReloadingStateStart,
						})
					} else if changeEvent.Name == filepath.Join(configDir, "..data") && changeEvent.Has(fsnotify.Create) {
						// the change for k8s configmap
						event.Emit(proxy.ConfigFileChangedEvent{
							ReloadingState: proxy.ReloadingStateStart,
						})
					}

				case err := <-watcher.Errors:
					log.Printf("File watcher error: %v", err)
				}
			}
		}()
	}

	// shutdown on signal
	go func() {
		sig := <-sigChan
		fmt.Printf("Received signal %v, shutting down...\n", sig)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
		defer cancel()

		if pm, ok := srv.Handler.(*proxy.ProxyManager); ok {
			pm.Shutdown()
		} else {
			fmt.Println("srv.Handler is not of type *proxy.ProxyManager")
		}

		if err := srv.Shutdown(ctx); err != nil {
			fmt.Printf("Server shutdown error: %v\n", err)
		}
		close(exitChan)
	}()

	// Start server
	fmt.Printf("FrogLLM listening on %s\n", *listenStr)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Fatal server error: %v\n", err)
		}
	}()

	// Set HF token if provided via command line
	if *hfToken != "" {
		go func() {
			// Wait a moment for the server to be ready
			time.Sleep(500 * time.Millisecond)
			setHFToken(*listenStr, *hfToken)
		}()
	}

	// Wait for exit signal
	<-exitChan
}

func debounce(interval time.Duration, f func()) func() {
	var timer *time.Timer
	return func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(interval, f)
	}
}

// selfHealReconfigure regenerates config.yaml from tracked folders using saved settings.
// Returns true if regeneration succeeded.
func selfHealReconfigure(configPath string) bool {
	// Instantiate a temporary ProxyManager-like helper by reusing functions in proxy package via HTTP API is not available here.
	// So we inline minimal logic: read folder DB, scan and run autosetup generator.
	// Load folder database
	dbPath := "model_folders.json"
	data, err := os.ReadFile(dbPath)
	if err != nil {
		fmt.Printf("Self-heal: no folder DB (%s): %v\n", dbPath, err)
		return false
	}
	var db struct {
		Folders []struct {
			Path    string
			Enabled bool
		} `json:"folders"`
	}
	if err := json.Unmarshal(data, &db); err != nil {
		fmt.Printf("Self-heal: invalid folder DB: %v\n", err)
		return false
	}
	var folders []string
	for _, f := range db.Folders {
		if f.Enabled {
			folders = append(folders, f.Path)
		}
	}
	if len(folders) == 0 {
		fmt.Println("Self-heal: no enabled folders; cannot regenerate")
		return false
	}

	// Load saved settings if present
	settingsPath := "settings.json"
	var opts autosetup.SetupOptions = autosetup.SetupOptions{
		EnableJinja:      true,
		ThroughputFirst:  true,
		MinContext:       16384,
		PreferredContext: 32768,
	}
	if sdata, err := os.ReadFile(settingsPath); err == nil {
		var s struct {
			Backend          string  `json:"backend"`
			VRAMGB           float64 `json:"vramGB"`
			RAMGB            float64 `json:"ramGB"`
			PreferredContext int     `json:"preferredContext"`
			ThroughputFirst  bool    `json:"throughputFirst"`
			EnableJinja      bool    `json:"enableJinja"`
		}
		if json.Unmarshal(sdata, &s) == nil {
			if s.EnableJinja {
				opts.EnableJinja = true
			} else {
				opts.EnableJinja = false
			}
			opts.ThroughputFirst = s.ThroughputFirst
			if s.PreferredContext > 0 {
				opts.PreferredContext = s.PreferredContext
			}
			if s.RAMGB > 0 {
				opts.ForceRAM = s.RAMGB
			}
			if s.VRAMGB > 0 {
				opts.ForceVRAM = s.VRAMGB
			}
			if s.Backend != "" {
				opts.ForceBackend = s.Backend
			}
		}
	}

	// Scan folders for models
	var allModels []autosetup.ModelInfo
	for _, p := range folders {
		models, err := autosetup.DetectModelsWithOptions(p, opts)
		if err != nil {
			fmt.Printf("Self-heal: scan failed for %s: %v\n", p, err)
			continue
		}
		allModels = append(allModels, models...)
	}
	if len(allModels) == 0 {
		fmt.Println("Self-heal: no models found in tracked folders")
		return false
	}

	system := autosetup.DetectSystem()
	_ = autosetup.EnhanceSystemInfo(&system)
	binariesDir := filepath.Join(".", "binaries")
	bin, err := autosetup.DownloadBinary(binariesDir, system, opts.ForceBackend)
	if err != nil {
		fmt.Printf("Self-heal: unable to ensure binary: %v\n", err)
		return false
	}
	gen := autosetup.NewConfigGenerator(folders[0], bin.Path, configPath, opts)
	gen.SetSystemInfo(&system)
	gen.SetAvailableVRAM(system.TotalVRAMGB)
	if err := gen.GenerateConfig(allModels); err != nil {
		fmt.Printf("Self-heal: generation failed: %v\n", err)
		return false
	}
	// Notify for live reload if server already running
	event.Emit(proxy.ConfigFileChangedEvent{ReloadingState: proxy.ReloadingStateStart})
	return true
}

// setHFToken sets the Hugging Face API token via HTTP request
func setHFToken(listenAddr, token string) {
	// Prepare the URL
	url := "http://localhost" + listenAddr + "/api/settings/hf-api-key"
	if strings.HasPrefix(listenAddr, ":") {
		url = "http://localhost" + listenAddr + "/api/settings/hf-api-key"
	} else {
		url = "http://" + listenAddr + "/api/settings/hf-api-key"
	}

	// Prepare the payload
	payload := map[string]string{
		"apiKey": token,
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("Warning: Failed to marshal HF token payload: %v\n", err)
		return
	}

	// Make the HTTP request
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("Warning: Failed to set HF token: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✅ Hugging Face API token set successfully")
	} else {
		fmt.Printf("Warning: Failed to set HF token (status: %d)\n", resp.StatusCode)
	}
}

// replaceLlamaServerInConfig replaces the llama-server binary path in the config file
func replaceLlamaServerInConfig(configPath, newLlamaServerPath string) error {
	// Read the config file
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read config file: %w", err)
	}

	// Convert to string for processing
	content := string(data)

	// Check if the new path exists
	if _, err := os.Stat(newLlamaServerPath); err != nil {
		return fmt.Errorf("llama-server binary not found at %s: %w", newLlamaServerPath, err)
	}

	// Get absolute path for the new llama-server
	absPath, err := filepath.Abs(newLlamaServerPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Replace various patterns of llama-server paths
	patterns := []string{
		// Match binaries/llama-server/build/bin/llama-server
		`binaries/llama-server/build/bin/llama-server`,
		// Match paths like /path/to/llama-server
		`[^\s"']+/llama-server`,
		// Match just llama-server
		`\bllama-server\b`,
	}

	replaced := false
	for _, pattern := range patterns {
		regex := regexp.MustCompile(pattern)
		if regex.MatchString(content) {
			content = regex.ReplaceAllString(content, absPath)
			replaced = true
		}
	}

	// If we have a macros section with llama-server-base, update it specifically
	macroPattern := regexp.MustCompile(`("llama-server-base":\s*>\s*\n\s*)([^\s\n]+)`)
	if macroPattern.MatchString(content) {
		content = macroPattern.ReplaceAllString(content, "${1}"+absPath)
		replaced = true
	}

	// Also handle llama-embed-base
	embedPattern := regexp.MustCompile(`("llama-embed-base":\s*>\s*\n\s*)([^\s\n]+)`)
	if embedPattern.MatchString(content) {
		content = embedPattern.ReplaceAllString(content, "${1}"+absPath)
		replaced = true
	}

	if !replaced {
		fmt.Println("Warning: No llama-server paths found in config to replace")
		// Still write the config in case it's a new format we don't recognize
	}

	// Write back the modified config
	if err := os.WriteFile(configPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	// Create a backup of the original config
	backupPath := configPath + ".backup." + time.Now().Format("20060102-150405")
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		fmt.Printf("Warning: Failed to create backup at %s: %v\n", backupPath, err)
	} else {
		fmt.Printf("📁 Original config backed up to: %s\n", backupPath)
	}

	return nil
}

// runBuildScript runs the build.py script to rebuild the application
func runBuildScript() error {
	// Check if build.py exists
	buildScript := "build.py"
	if _, err := os.Stat(buildScript); err != nil {
		if os.IsNotExist(err) {
			// Try alternative build command
			fmt.Println("build.py not found, trying direct go build...")
			return runGoBuild()
		}
		return fmt.Errorf("error checking build script: %w", err)
	}

	// Run python3 build.py
	cmd := exec.Command("python3", buildScript)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Println("Running: python3 build.py")
	if err := cmd.Run(); err != nil {
		// Try python if python3 fails
		fmt.Println("python3 failed, trying python...")
		cmd = exec.Command("python", buildScript)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		cmd.Stdin = os.Stdin

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build script failed: %w", err)
		}
	}

	return nil
}

// runGoBuild runs go build directly as a fallback
func runGoBuild() error {
	cmd := exec.Command("go", "build", "-o", "frogllm", ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	fmt.Println("Running: go build -o frogllm .")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}

	return nil
}
