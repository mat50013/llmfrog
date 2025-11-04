package proxy

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

// ModelLoadRequest represents a request to load a model
type ModelLoadRequest struct {
	ModelID    string   `json:"model_id"`
	GPUIDs     []int    `json:"gpu_ids,omitempty"`
	AutoUnload bool     `json:"auto_unload"`
}

// ModelUnloadRequest represents a request to unload a model
type ModelUnloadRequest struct {
	ModelID string `json:"model_id"`
}

// LoadedModelInfo represents information about a loaded model
type LoadedModelInfo struct {
	ModelID       string    `json:"model_id"`
	Name          string    `json:"name"`
	Status        string    `json:"status"`
	LoadedAt      time.Time `json:"loaded_at"`
	LastUsed      time.Time `json:"last_used"`
	RequestCount  int64     `json:"request_count"`
	VRAMUsage     float64   `json:"vram_usage_gb,omitempty"`
	ProcessID     int       `json:"process_id,omitempty"`
	Port          int       `json:"port,omitempty"`
}

// ModelUsageTracker tracks model usage for LRU eviction
type ModelUsageTracker struct {
	mu       sync.RWMutex
	usage    map[string]time.Time
	requests map[string]int64
	loadTime map[string]time.Time
}

var modelTracker = &ModelUsageTracker{
	usage:    make(map[string]time.Time),
	requests: make(map[string]int64),
	loadTime: make(map[string]time.Time),
}

// UpdateModelUsage updates the last used time for a model
func (m *ModelUsageTracker) UpdateModelUsage(modelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.usage[modelID] = time.Now()
	m.requests[modelID]++
}

// RecordModelLoad records when a model was loaded
func (m *ModelUsageTracker) RecordModelLoad(modelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	m.loadTime[modelID] = now
	m.usage[modelID] = now
	if _, exists := m.requests[modelID]; !exists {
		m.requests[modelID] = 0
	}
}

// GetLRUModels returns models sorted by last used time (oldest first)
func (m *ModelUsageTracker) GetLRUModels() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type modelUsage struct {
		id       string
		lastUsed time.Time
	}

	models := make([]modelUsage, 0, len(m.usage))
	for id, lastUsed := range m.usage {
		models = append(models, modelUsage{id: id, lastUsed: lastUsed})
	}

	// Sort by last used time (oldest first)
	sort.Slice(models, func(i, j int) bool {
		return models[i].lastUsed.Before(models[j].lastUsed)
	})

	result := make([]string, len(models))
	for i, m := range models {
		result[i] = m.id
	}
	return result
}

// apiV1LoadModel handles POST /v1/models/load
func (pm *ProxyManager) apiV1LoadModel(c *gin.Context) {
	var req ModelLoadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ModelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model_id is required"})
		return
	}

	// Check if model needs to be downloaded first
	modelPath := pm.resolveModelPath(req.ModelID)
	if modelPath == "" && req.AutoUnload {
		// Try to download the model
		if err := pm.autoDownloadModel(c, req.ModelID); err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": fmt.Sprintf("Model %s not found locally and download failed: %v", req.ModelID, err),
			})
			return
		}
		// Reload config to pick up new model (not deferring save since this is a separate API call)
		_, _ = pm.reloadConfigForNewModel(req.ModelID, false)
	}

	// If auto-unload is enabled, check VRAM and unload LRU models if needed
	if req.AutoUnload {
		requiredVRAM := pm.estimateModelVRAM(req.ModelID)
		availableVRAM := pm.getAvailableVRAM()

		if requiredVRAM > availableVRAM {
			// Need to free up VRAM
			freedVRAM := 0.0
			lruModels := modelTracker.GetLRUModels()

			for _, modelToUnload := range lruModels {
				if modelToUnload == req.ModelID {
					continue // Don't unload the model we're trying to load
				}

				modelVRAM := pm.getModelVRAMUsage(modelToUnload)
				if err := pm.unloadSpecificModel(modelToUnload); err == nil {
					freedVRAM += modelVRAM
					pm.proxyLogger.Infof("Auto-unloaded model %s to free %.2f GB VRAM", modelToUnload, modelVRAM)

					if freedVRAM >= (requiredVRAM - availableVRAM) {
						break // Freed enough VRAM
					}
				}
			}
		}
	}

	// Now load the requested model
	if err := pm.loadSpecificModel(req.ModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to load model %s: %v", req.ModelID, err),
		})
		return
	}

	// Record the model load
	modelTracker.RecordModelLoad(req.ModelID)

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"message": fmt.Sprintf("Model %s loaded successfully", req.ModelID),
		"model_id": req.ModelID,
	})
}

// apiV1UnloadModel handles POST /v1/models/unload
func (pm *ProxyManager) apiV1UnloadModel(c *gin.Context) {
	var req ModelUnloadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ModelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model_id is required"})
		return
	}

	if err := pm.unloadSpecificModel(req.ModelID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to unload model %s: %v", req.ModelID, err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"message": fmt.Sprintf("Model %s unloaded successfully", req.ModelID),
		"model_id": req.ModelID,
	})
}

// apiV1GetLoadedModels handles GET /v1/models/loaded
func (pm *ProxyManager) apiV1GetLoadedModels(c *gin.Context) {
	pm.Lock()
	defer pm.Unlock()

	loadedModels := make([]LoadedModelInfo, 0)

	for modelID, processGroup := range pm.processGroups {
		if processGroup == nil {
			continue
		}

		for _, process := range processGroup.processes {
			if process == nil || process.CurrentState() != StateReady {
				continue
			}

			modelConfig := pm.config.Models[modelID]
			modelInfo := LoadedModelInfo{
				ModelID:      modelID,
				Name:         modelConfig.Name,
				Status:       "ready",
				LoadedAt:     modelTracker.loadTime[modelID],
				LastUsed:     modelTracker.usage[modelID],
				RequestCount: modelTracker.requests[modelID],
			}

			// Add process details if available
			if process.cmd != nil && process.cmd.Process != nil {
				modelInfo.ProcessID = process.cmd.Process.Pid
			}
			// Port information would need to be extracted from process struct
			// For now, using a default value
			modelInfo.Port = 8000

			// Estimate VRAM usage (this would need proper implementation)
			modelInfo.VRAMUsage = pm.getModelVRAMUsage(modelID)

			loadedModels = append(loadedModels, modelInfo)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"models": loadedModels,
		"total": len(loadedModels),
	})
}

// Helper methods that need to be implemented in ProxyManager

func (pm *ProxyManager) resolveModelPath(modelID string) string {
	// Check if model exists locally
	// This should check config and filesystem
	// Return empty string if not found
	return ""
}

func (pm *ProxyManager) estimateModelVRAM(modelID string) float64 {
	// Estimate VRAM requirement for a model
	// Based on model size and quantization
	// Default estimate: 8GB for 7B models, 16GB for 13B, etc.
	return 8.0
}

func (pm *ProxyManager) getAvailableVRAM() float64 {
	// Get currently available VRAM across all GPUs
	// Should query actual GPU stats
	return 24.0 // Placeholder
}

func (pm *ProxyManager) getModelVRAMUsage(modelID string) float64 {
	// Get actual VRAM usage of a loaded model
	// Should query from process metrics
	return 8.0 // Placeholder
}

func (pm *ProxyManager) loadSpecificModel(modelID string) error {
	// Load a specific model
	// This should integrate with existing model loading logic
	return fmt.Errorf("not implemented")
}

func (pm *ProxyManager) unloadSpecificModel(modelID string) error {
	// Unload a specific model
	// This should integrate with existing model unloading logic
	pm.Lock()
	defer pm.Unlock()

	processGroup, exists := pm.processGroups[modelID]
	if !exists {
		return fmt.Errorf("model %s is not loaded", modelID)
	}

	processGroup.StopProcesses(StopImmediately)
	delete(pm.processGroups, modelID)
	return nil
}