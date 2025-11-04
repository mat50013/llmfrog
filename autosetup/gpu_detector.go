package autosetup

import (
	"fmt"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// GPUDevice represents information about a single GPU device
type GPUDevice struct {
	Index       int     `json:"index"`
	Name        string  `json:"name"`
	UUID        string  `json:"uuid"`
	MemoryTotal float64 `json:"memoryTotal"` // In GB
	MemoryFree  float64 `json:"memoryFree"`  // In GB
	MemoryUsed  float64 `json:"memoryUsed"`  // In GB
	Utilization int     `json:"utilization"` // GPU utilization percentage
	Temperature int     `json:"temperature"` // Temperature in Celsius
	PowerDraw   float64 `json:"powerDraw"`   // Power draw in Watts
	PowerLimit  float64 `json:"powerLimit"`  // Power limit in Watts
	Driver      string  `json:"driver"`      // Driver version
}

// MultiGPUInfo represents information about all GPUs in the system
type MultiGPUInfo struct {
	GPUs         []GPUDevice `json:"gpus"`
	TotalGPUs    int       `json:"totalGPUs"`
	TotalMemory  float64   `json:"totalMemory"`  // Total memory across all GPUs in GB
	TotalFree    float64   `json:"totalFree"`    // Total free memory across all GPUs in GB
	Backend      string    `json:"backend"`      // cuda, rocm, metal, vulkan, etc.
	DriverVersion string   `json:"driverVersion"`
}

// DetectAllGPUs detects all GPUs in the system and returns their information
func DetectAllGPUs() (*MultiGPUInfo, error) {
	switch runtime.GOOS {
	case "linux", "windows":
		// Try NVIDIA first
		if info, err := detectNvidiaGPUs(); err == nil {
			return info, nil
		}

		// Try AMD ROCm
		if info, err := detectAMDGPUs(); err == nil {
			return info, nil
		}

		// Try Intel
		if info, err := detectIntelGPUs(); err == nil {
			return info, nil
		}

	case "darwin":
		// macOS - Metal
		if info, err := detectMetalGPUs(); err == nil {
			return info, nil
		}
	}

	return nil, fmt.Errorf("no GPUs detected or GPU drivers not available")
}

// detectNvidiaGPUs detects NVIDIA GPUs using nvidia-smi
func detectNvidiaGPUs() (*MultiGPUInfo, error) {
	// Check if nvidia-smi is available
	cmd := exec.Command("nvidia-smi", "--query-gpu=index,name,uuid,memory.total,memory.free,memory.used,utilization.gpu,temperature.gpu,power.draw,power.limit,driver_version", "--format=csv,noheader,nounits")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nvidia-smi not available: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) == 0 {
		return nil, fmt.Errorf("no NVIDIA GPUs found")
	}

	info := &MultiGPUInfo{
		GPUs:    make([]GPUDevice, 0),
		Backend: "cuda",
	}

	for _, line := range lines {
		parts := strings.Split(line, ", ")
		if len(parts) < 11 {
			continue
		}

		gpu := GPUDevice{}

		// Parse index
		if idx, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
			gpu.Index = idx
		}

		// Name
		gpu.Name = strings.TrimSpace(parts[1])

		// UUID
		gpu.UUID = strings.TrimSpace(parts[2])

		// Memory (convert from MB to GB)
		if memTotal, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64); err == nil {
			gpu.MemoryTotal = memTotal / 1024.0
		}
		if memFree, err := strconv.ParseFloat(strings.TrimSpace(parts[4]), 64); err == nil {
			gpu.MemoryFree = memFree / 1024.0
		}
		if memUsed, err := strconv.ParseFloat(strings.TrimSpace(parts[5]), 64); err == nil {
			gpu.MemoryUsed = memUsed / 1024.0
		}

		// Utilization
		if util, err := strconv.Atoi(strings.TrimSpace(parts[6])); err == nil {
			gpu.Utilization = util
		}

		// Temperature
		if temp, err := strconv.Atoi(strings.TrimSpace(parts[7])); err == nil {
			gpu.Temperature = temp
		}

		// Power
		if power, err := strconv.ParseFloat(strings.TrimSpace(parts[8]), 64); err == nil {
			gpu.PowerDraw = power
		}
		if powerLimit, err := strconv.ParseFloat(strings.TrimSpace(parts[9]), 64); err == nil {
			gpu.PowerLimit = powerLimit
		}

		// Driver
		gpu.Driver = strings.TrimSpace(parts[10])
		if info.DriverVersion == "" {
			info.DriverVersion = gpu.Driver
		}

		info.GPUs = append(info.GPUs, gpu)
		info.TotalMemory += gpu.MemoryTotal
		info.TotalFree += gpu.MemoryFree
	}

	info.TotalGPUs = len(info.GPUs)

	if info.TotalGPUs == 0 {
		return nil, fmt.Errorf("no NVIDIA GPUs detected")
	}

	return info, nil
}

// detectAMDGPUs detects AMD GPUs using rocm-smi
func detectAMDGPUs() (*MultiGPUInfo, error) {
	// Check if rocm-smi is available
	cmd := exec.Command("rocm-smi", "--showmeminfo", "vram", "--csv")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("rocm-smi not available: %v", err)
	}

	info := &MultiGPUInfo{
		GPUs:    make([]GPUDevice, 0),
		Backend: "rocm",
	}

	// Parse ROCm output (simplified - would need more comprehensive parsing)
	lines := strings.Split(string(output), "\n")
	gpuIndex := 0

	for _, line := range lines {
		if strings.Contains(line, "GPU") && strings.Contains(line, "VRAM") {
			gpu := GPUDevice{
				Index: gpuIndex,
				Name:  fmt.Sprintf("AMD GPU %d", gpuIndex),
			}

			// Parse memory info from line
			// This is simplified - actual implementation would need proper CSV parsing
			parts := strings.Fields(line)
			for i, part := range parts {
				if strings.Contains(part, "Total") && i+1 < len(parts) {
					if memTotal, err := strconv.ParseFloat(parts[i+1], 64); err == nil {
						gpu.MemoryTotal = memTotal / 1024.0 // Convert MB to GB
					}
				}
			}

			info.GPUs = append(info.GPUs, gpu)
			info.TotalMemory += gpu.MemoryTotal
			gpuIndex++
		}
	}

	info.TotalGPUs = len(info.GPUs)

	if info.TotalGPUs == 0 {
		return nil, fmt.Errorf("no AMD GPUs detected")
	}

	return info, nil
}

// detectIntelGPUs detects Intel GPUs
func detectIntelGPUs() (*MultiGPUInfo, error) {
	// Intel GPU detection would go here
	// This would use intel_gpu_top or similar tools
	return nil, fmt.Errorf("Intel GPU detection not yet implemented")
}

// detectMetalGPUs detects Metal GPUs on macOS
func detectMetalGPUs() (*MultiGPUInfo, error) {
	// Use system_profiler to get GPU information
	cmd := exec.Command("system_profiler", "SPDisplaysDataType", "-json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get GPU info on macOS: %v", err)
	}

	// Parse the JSON output to extract GPU information
	// This is simplified - would need proper JSON parsing
	info := &MultiGPUInfo{
		GPUs:    make([]GPUDevice, 0),
		Backend: "metal",
	}

	// Check for common GPU patterns in output
	outputStr := string(output)
	gpuIndex := 0

	// Look for GPU entries (simplified detection)
	if strings.Contains(outputStr, "Apple M1") || strings.Contains(outputStr, "Apple M2") || strings.Contains(outputStr, "Apple M3") {
		gpu := GPUDevice{
			Index: gpuIndex,
			Name:  "Apple Silicon GPU",
		}

		// Get unified memory info
		cmd := exec.Command("sysctl", "-n", "hw.memsize")
		if output, err := cmd.Output(); err == nil {
			if memBytes, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64); err == nil {
				// For Apple Silicon, VRAM is part of unified memory
				// Assume up to 75% can be used for GPU
				gpu.MemoryTotal = (memBytes / (1024 * 1024 * 1024)) * 0.75
				gpu.MemoryFree = gpu.MemoryTotal * 0.8 // Estimate
			}
		}

		info.GPUs = append(info.GPUs, gpu)
		info.TotalMemory += gpu.MemoryTotal
		info.TotalFree += gpu.MemoryFree
	}

	info.TotalGPUs = len(info.GPUs)

	if info.TotalGPUs == 0 {
		return nil, fmt.Errorf("no Metal GPUs detected")
	}

	return info, nil
}

// GetGPUStats returns current GPU statistics (for monitoring)
func GetGPUStats() (*MultiGPUInfo, error) {
	return DetectAllGPUs()
}

// GetGPUMemoryForIndex returns memory info for a specific GPU index
func GetGPUMemoryForIndex(index int) (total, free, used float64, err error) {
	info, err := DetectAllGPUs()
	if err != nil {
		return 0, 0, 0, err
	}

	if index < 0 || index >= len(info.GPUs) {
		return 0, 0, 0, fmt.Errorf("GPU index %d out of range (0-%d)", index, len(info.GPUs)-1)
	}

	gpuDevice := info.GPUs[index]
	return gpuDevice.MemoryTotal, gpuDevice.MemoryFree, gpuDevice.MemoryUsed, nil
}