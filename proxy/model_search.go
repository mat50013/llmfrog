package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ModelSearchResult represents a searchable model with all necessary info
type ModelSearchResult struct {
	ID           string  `json:"id"`           // repo:filename or repo:quantization format
	Name         string  `json:"name"`
	Quantization string  `json:"quantization"`
	SizeGB       float64 `json:"size_gb"`
	RequiresAuth bool    `json:"requires_auth"`
	Repo         string  `json:"repo"`
	File         string  `json:"file"`
	Downloads    int     `json:"downloads,omitempty"`
	Likes        int     `json:"likes,omitempty"`
	Tags         []string `json:"tags,omitempty"`
}

// HuggingFaceSearchResponse represents the HF API response
type HuggingFaceSearchResponse struct {
	ID          string   `json:"id"`
	ModelID     string   `json:"modelId"`
	Author      string   `json:"author,omitempty"`
	Downloads   int      `json:"downloads"`
	Likes       int      `json:"likes"`
	Tags        []string `json:"tags"`
	Pipeline    string   `json:"pipeline_tag,omitempty"`
	Private     bool     `json:"private"`
	Gated       bool     `json:"gated,omitempty"`
	Siblings    []HFSibling `json:"siblings,omitempty"`
}

type HFSibling struct {
	RFilename string `json:"rfilename"`
	Size      int64  `json:"size,omitempty"`
}

// apiV1SearchModels provides a unified model search endpoint
func (pm *ProxyManager) apiV1SearchModels(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Query parameter 'q' is required",
		})
		return
	}

	includeRestricted := c.Query("include_restricted") == "true"
	limit := c.DefaultQuery("limit", "20")

	// Get HF token from system settings or header
	hfToken := c.GetHeader("HF-Token")
	if hfToken == "" {
		hfToken = c.GetHeader("X-HF-Token")
	}

	// Fall back to stored HF token if available
	if hfToken == "" {
		settings := pm.getSystemSettings()
		if settings != nil && settings.HuggingFaceApiKey != "" {
			hfToken = settings.HuggingFaceApiKey
		}
	}

	// Search HuggingFace for models
	searchURL := "https://huggingface.co/api/models"
	params := url.Values{
		"search": {query},
		"limit":  {limit},
		"filter": {"gguf"},
		"full":   {"true"},
	}

	if includeRestricted && hfToken != "" {
		params.Add("include_gated", "true")
	}

	req, err := http.NewRequest("GET", searchURL+"?"+params.Encode(), nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create search request",
		})
		return
	}

	if hfToken != "" {
		req.Header.Set("Authorization", "Bearer "+hfToken)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to search models",
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("HuggingFace API returned status %d", resp.StatusCode),
		})
		return
	}

	var hfModels []HuggingFaceSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&hfModels); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to parse search results",
		})
		return
	}

	// Convert to our unified format
	results := make([]ModelSearchResult, 0)
	for _, hfModel := range hfModels {
		// Skip non-GGUF models
		hasGGUF := false
		for _, sibling := range hfModel.Siblings {
			if strings.HasSuffix(strings.ToLower(sibling.RFilename), ".gguf") {
				hasGGUF = true
				// Create a result for each GGUF file
				quantization := extractQuantization(sibling.RFilename)
				result := ModelSearchResult{
					ID:           fmt.Sprintf("%s:%s", hfModel.ID, sibling.RFilename),
					Name:         formatModelName(hfModel.ID, sibling.RFilename),
					Quantization: quantization,
					SizeGB:       float64(sibling.Size) / (1024 * 1024 * 1024),
					RequiresAuth: hfModel.Gated || hfModel.Private,
					Repo:         hfModel.ID,
					File:         sibling.RFilename,
					Downloads:    hfModel.Downloads,
					Likes:        hfModel.Likes,
					Tags:         hfModel.Tags,
				}
				results = append(results, result)
			}
		}

		// If no siblings info but has GGUF tag, include a generic entry
		if !hasGGUF && len(hfModel.Siblings) == 0 {
			for _, tag := range hfModel.Tags {
				if strings.ToLower(tag) == "gguf" {
					results = append(results, ModelSearchResult{
						ID:           hfModel.ID,
						Name:         hfModel.ID,
						RequiresAuth: hfModel.Gated || hfModel.Private,
						Repo:         hfModel.ID,
						Downloads:    hfModel.Downloads,
						Likes:        hfModel.Likes,
						Tags:         hfModel.Tags,
					})
					break
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"models": results,
		"total":  len(results),
	})
}

// extractQuantization extracts quantization from filename
func extractQuantization(filename string) string {
	filename = strings.ToUpper(filename)

	// Common quantization patterns
	quantizations := []string{
		"Q2_K", "Q3_K_S", "Q3_K_M", "Q3_K_L",
		"Q4_0", "Q4_1", "Q4_K_S", "Q4_K_M",
		"Q5_0", "Q5_1", "Q5_K_S", "Q5_K_M",
		"Q6_K", "Q8_0", "F16", "F32",
		"IQ1_S", "IQ2_XXS", "IQ2_XS", "IQ2_S",
		"IQ3_XXS", "IQ3_XS", "IQ3_S", "IQ3_M",
		"IQ4_XS", "IQ4_NL",
	}

	for _, q := range quantizations {
		if strings.Contains(filename, q) {
			return q
		}
	}

	return "Unknown"
}

// formatModelName creates a readable name from repo and filename
func formatModelName(repo, filename string) string {
	// Remove extension
	name := strings.TrimSuffix(filename, filepath.Ext(filename))

	// Extract model name from repo
	parts := strings.Split(repo, "/")
	if len(parts) > 1 {
		// Remove common prefixes/suffixes
		modelName := strings.TrimSuffix(parts[1], "-GGUF")
		modelName = strings.TrimSuffix(modelName, "-gguf")

		// Don't duplicate if filename already contains the model name
		if !strings.Contains(strings.ToLower(name), strings.ToLower(modelName)) {
			name = modelName + " " + name
		}
	}

	// Clean up the name
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Remove duplicate spaces
	for strings.Contains(name, "  ") {
		name = strings.ReplaceAll(name, "  ", " ")
	}

	return strings.TrimSpace(name)
}

// getSystemSettings retrieves system settings with HF token
func (pm *ProxyManager) getSystemSettings() *SystemSettings {
	// This should be integrated with your existing settings management
	// For now, returning a placeholder
	settings, _ := pm.loadSystemSettings()
	return settings
}