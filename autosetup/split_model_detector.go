package autosetup

import (
	"fmt"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// SplitModelInfo represents a multi-part model
type SplitModelInfo struct {
	BaseName      string       // Base name of the model (without part numbers)
	Parts         []ModelInfo  // All parts of the split model
	TotalParts    int          // Total number of parts
	IsComplete    bool         // Whether all parts are present
	CombinedPath  string       // Path to use for the combined model
	Quantization  string       // Quantization type (Q4_K_M, etc)
	FolderPath    string       // Folder containing the model parts
}

// DetectSplitModels identifies and groups split GGUF models
func DetectSplitModels(models []ModelInfo) ([]SplitModelInfo, []ModelInfo) {
	// Regular expressions for detecting split models
	splitPatterns := []*regexp.Regexp{
		regexp.MustCompile(`(.+?)-(\d{5})-of-(\d{5})\.gguf$`), // pattern: model-00001-of-00003.gguf
		regexp.MustCompile(`(.+?)\.gguf\.part(\d+)of(\d+)$`),   // pattern: model.gguf.part1of3
		regexp.MustCompile(`(.+?)_part_(\d+)_of_(\d+)\.gguf$`), // pattern: model_part_1_of_3.gguf
	}

	splitModelsMap := make(map[string]*SplitModelInfo)
	regularModels := []ModelInfo{}
	processedPaths := make(map[string]bool)

	for _, model := range models {
		// Skip if already processed as part of a split model
		if processedPaths[model.Path] {
			continue
		}

		// Check if this is part of a split model
		isSplit := false
		for _, pattern := range splitPatterns {
			matches := pattern.FindStringSubmatch(filepath.Base(model.Path))
			if len(matches) > 0 {
				isSplit = true
				baseName := matches[1]
				partNum := matches[2]
				totalParts := matches[3]

				// Create a unique key for this split model group
				modelDir := filepath.Dir(model.Path)
				groupKey := filepath.Join(modelDir, baseName)

				// Initialize or update the split model info
				if _, exists := splitModelsMap[groupKey]; !exists {
					splitModelsMap[groupKey] = &SplitModelInfo{
						BaseName:     baseName,
						Parts:        []ModelInfo{},
						TotalParts:   parseInt(totalParts),
						FolderPath:   modelDir,
						Quantization: extractQuantization(baseName),
					}
				}

				// Add this part to the split model
				splitInfo := splitModelsMap[groupKey]
				splitInfo.Parts = append(splitInfo.Parts, model)
				processedPaths[model.Path] = true

				// If this is part 1, use it as the primary path
				if partNum == "00001" || partNum == "1" {
					splitInfo.CombinedPath = model.Path
				}
				break
			}
		}

		if !isSplit {
			// Check if model is in a quantization folder structure
			dir := filepath.Dir(model.Path)
			dirName := filepath.Base(dir)

			// Common quantization folder patterns
			quantPatterns := []string{"Q2_K", "Q3_K", "Q4_K", "Q5_K", "Q6_K", "Q8_0", "F16", "F32"}
			isQuantFolder := false

			for _, pattern := range quantPatterns {
				if strings.Contains(strings.ToUpper(dirName), pattern) {
					isQuantFolder = true
					model.Quantization = dirName
					break
				}
			}

			// If in a quant folder, check if there are other parts in the same folder
			if isQuantFolder {
				// Look for other GGUF files in the same directory that might be parts
				// This is handled by the folder scanning logic
				model.Name = fmt.Sprintf("%s (%s)", filepath.Base(filepath.Dir(dir)), dirName)
			}

			regularModels = append(regularModels, model)
		}
	}

	// Convert map to slice and check completeness
	splitModels := []SplitModelInfo{}
	for _, splitInfo := range splitModelsMap {
		// Sort parts by filename for consistent ordering
		sort.Slice(splitInfo.Parts, func(i, j int) bool {
			return splitInfo.Parts[i].Path < splitInfo.Parts[j].Path
		})

		// Check if all parts are present
		splitInfo.IsComplete = len(splitInfo.Parts) == splitInfo.TotalParts

		// If no primary path was set, use the first part
		if splitInfo.CombinedPath == "" && len(splitInfo.Parts) > 0 {
			splitInfo.CombinedPath = splitInfo.Parts[0].Path
		}

		splitModels = append(splitModels, *splitInfo)
	}

	return splitModels, regularModels
}

// CombineSplitModels converts split models into single ModelInfo entries
func CombineSplitModels(splitModels []SplitModelInfo, regularModels []ModelInfo) []ModelInfo {
	allModels := regularModels

	for _, split := range splitModels {
		if !split.IsComplete {
			fmt.Printf("⚠️  Warning: Split model %s is incomplete (%d/%d parts found)\n",
				split.BaseName, len(split.Parts), split.TotalParts)
			continue
		}

		// Create a combined model entry
		combinedModel := ModelInfo{
			Name:         split.BaseName,
			Path:         split.CombinedPath, // Use the first part as the entry point
			Quantization: split.Quantization,
		}

		// Copy metadata from the first part
		if len(split.Parts) > 0 {
			firstPart := split.Parts[0]
			combinedModel.Size = firstPart.Size
			combinedModel.IsInstruct = firstPart.IsInstruct
			combinedModel.IsEmbedding = firstPart.IsEmbedding
			combinedModel.ContextLength = firstPart.ContextLength
			combinedModel.EmbeddingSize = firstPart.EmbeddingSize
			combinedModel.NumLayers = firstPart.NumLayers
			combinedModel.IsMoE = firstPart.IsMoE
		}

		// Add size information
		// totalSize := int64(0)
		// for _, part := range split.Parts {
		// 	// Parse size from part.Size string if needed
		// 	// This would need proper size parsing logic
		// }

		allModels = append(allModels, combinedModel)
	}

	return allModels
}

// extractQuantization extracts quantization type from filename
func extractQuantization(filename string) string {
	upper := strings.ToUpper(filename)
	quantPatterns := []string{
		"Q2_K", "Q3_K_S", "Q3_K_M", "Q3_K_L",
		"Q4_0", "Q4_1", "Q4_K_S", "Q4_K_M",
		"Q5_0", "Q5_1", "Q5_K_S", "Q5_K_M",
		"Q6_K", "Q8_0", "F16", "F32",
	}

	for _, pattern := range quantPatterns {
		if strings.Contains(upper, pattern) {
			return pattern
		}
	}

	return ""
}

// parseInt safely parses a string to int
func parseInt(s string) int {
	var result int
	fmt.Sscanf(s, "%d", &result)
	return result
}

// DetectModelsInFolders enhances model detection to handle folder structures
func DetectModelsInFolders(modelsDir string) ([]ModelInfo, error) {
	// First do regular detection
	models, err := DetectModels(modelsDir)
	if err != nil {
		return nil, err
	}

	// Group models by their parent directory
	modelsByDir := make(map[string][]ModelInfo)
	for _, model := range models {
		dir := filepath.Dir(model.Path)
		modelsByDir[dir] = append(modelsByDir[dir], model)
	}

	// Check for quantization folders
	finalModels := []ModelInfo{}
	processedDirs := make(map[string]bool)

	for dir, dirModels := range modelsByDir {
		dirName := filepath.Base(dir)
		parentDir := filepath.Dir(dir)
		parentDirName := filepath.Base(parentDir)

		// Check if this is a quantization folder (e.g., "Q4_K_M" folder)
		if isQuantizationFolder(dirName) {
			// This is a quant folder, group all models in it
			if !processedDirs[dir] {
				// Check for split models in this folder
				splitModels, regularModels := DetectSplitModels(dirModels)

				// Combine split models
				combined := CombineSplitModels(splitModels, regularModels)

				// Update model names to include parent model name
				for i := range combined {
					combined[i].Name = fmt.Sprintf("%s-%s", parentDirName, dirName)
					combined[i].Quantization = dirName
				}

				finalModels = append(finalModels, combined...)
				processedDirs[dir] = true
			}
		} else {
			// Regular folder, process normally
			if !processedDirs[dir] {
				splitModels, regularModels := DetectSplitModels(dirModels)
				combined := CombineSplitModels(splitModels, regularModels)
				finalModels = append(finalModels, combined...)
				processedDirs[dir] = true
			}
		}
	}

	return finalModels, nil
}

// isQuantizationFolder checks if a folder name represents a quantization type
func isQuantizationFolder(name string) bool {
	upper := strings.ToUpper(name)
	quantPatterns := []string{
		"Q2_K", "Q3_K", "Q4_K", "Q5_K", "Q6_K", "Q8_0",
		"Q4_0", "Q4_1", "Q5_0", "Q5_1",
		"F16", "F32", "IQ", "GGUF",
	}

	for _, pattern := range quantPatterns {
		if strings.Contains(upper, pattern) {
			return true
		}
	}

	return false
}