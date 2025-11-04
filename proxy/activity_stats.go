package proxy

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// ActivityStats represents persistent statistics for model usage
type ActivityStats struct {
	ModelID          string    `json:"model_id"`
	TotalTokens      int64     `json:"total_tokens"`
	PromptTokens     int64     `json:"prompt_tokens"`
	CompletionTokens int64     `json:"completion_tokens"`
	RequestCount     int64     `json:"request_count"`
	LastUsed         time.Time `json:"last_used"`
	FirstUsed        time.Time `json:"first_used"`
	TotalDurationMs  int64     `json:"total_duration_ms"`
}

// ActivityStatsManager handles persistent statistics storage
type ActivityStatsManager struct {
	mu         sync.RWMutex
	stats      map[string]*ActivityStats
	globalStats *ActivityStats
	filePath   string
}

// NewActivityStatsManager creates a new activity stats manager
func NewActivityStatsManager(filePath string) *ActivityStatsManager {
	if filePath == "" {
		filePath = "activity_stats.json"
	}

	manager := &ActivityStatsManager{
		stats:    make(map[string]*ActivityStats),
		filePath: filePath,
		globalStats: &ActivityStats{
			ModelID:   "_global_",
			FirstUsed: time.Now(),
		},
	}

	// Load existing stats from file
	manager.loadFromFile()

	return manager
}

// loadFromFile loads statistics from persistent storage
func (m *ActivityStatsManager) loadFromFile() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet, that's fine
			return nil
		}
		return err
	}

	var savedData struct {
		Stats      map[string]*ActivityStats `json:"stats"`
		GlobalStats *ActivityStats           `json:"global_stats"`
	}

	if err := json.Unmarshal(data, &savedData); err != nil {
		return err
	}

	if savedData.Stats != nil {
		m.stats = savedData.Stats
	}
	if savedData.GlobalStats != nil {
		m.globalStats = savedData.GlobalStats
	}

	return nil
}

// SaveToFile persists statistics to storage
func (m *ActivityStatsManager) SaveToFile() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	data := struct {
		Stats      map[string]*ActivityStats `json:"stats"`
		GlobalStats *ActivityStats           `json:"global_stats"`
	}{
		Stats:      m.stats,
		GlobalStats: m.globalStats,
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(m.filePath, jsonData, 0644)
	if err == nil {
		// Log successful save (for debugging)
		// fmt.Printf("[DEBUG] Activity stats saved to %s (global tokens: %d)\n", m.filePath, m.globalStats.TotalTokens)
	}
	return err
}

// RecordActivity records activity for a model
func (m *ActivityStatsManager) RecordActivity(modelID string, promptTokens, completionTokens int, durationMs int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	totalTokens := int64(promptTokens + completionTokens)

	// Update per-model stats
	stats, exists := m.stats[modelID]
	if !exists {
		stats = &ActivityStats{
			ModelID:   modelID,
			FirstUsed: now,
		}
		m.stats[modelID] = stats
	}

	stats.TotalTokens += totalTokens
	stats.PromptTokens += int64(promptTokens)
	stats.CompletionTokens += int64(completionTokens)
	stats.RequestCount++
	stats.LastUsed = now
	stats.TotalDurationMs += int64(durationMs)

	// Update global stats
	m.globalStats.TotalTokens += totalTokens
	m.globalStats.PromptTokens += int64(promptTokens)
	m.globalStats.CompletionTokens += int64(completionTokens)
	m.globalStats.RequestCount++
	m.globalStats.LastUsed = now
	m.globalStats.TotalDurationMs += int64(durationMs)

	// Save to file (could be throttled in production)
	go m.SaveToFile()
}

// GetStats returns a copy of all statistics
func (m *ActivityStatsManager) GetStats() map[string]*ActivityStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*ActivityStats)
	for k, v := range m.stats {
		// Create a copy
		statsCopy := *v
		result[k] = &statsCopy
	}

	// Include global stats
	globalCopy := *m.globalStats
	result["_global_"] = &globalCopy

	return result
}

// GetModelStats returns statistics for a specific model
func (m *ActivityStatsManager) GetModelStats(modelID string) (*ActivityStats, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats, exists := m.stats[modelID]
	if !exists {
		return nil, false
	}

	// Return a copy
	statsCopy := *stats
	return &statsCopy, true
}

// GetGlobalStats returns global statistics
func (m *ActivityStatsManager) GetGlobalStats() *ActivityStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// Return a copy
	statsCopy := *m.globalStats
	return &statsCopy
}

// ResetStats resets statistics for a model or all models
func (m *ActivityStatsManager) ResetStats(modelID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if modelID == "" {
		// Reset all stats
		m.stats = make(map[string]*ActivityStats)
		m.globalStats = &ActivityStats{
			ModelID:   "_global_",
			FirstUsed: time.Now(),
		}
	} else {
		// Reset specific model stats
		delete(m.stats, modelID)
	}

	// Save to file
	go m.SaveToFile()
}