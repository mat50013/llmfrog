package proxy

import (
	"fmt"
	"net/http"
	"sync"
)

type ProcessGroup struct {
	sync.Mutex

	config     Config
	id         string
	swap       bool
	exclusive  bool
	persistent bool

	proxyLogger    *LogMonitor
	upstreamLogger *LogMonitor

	// map of current processes
	processes       map[string]*Process
	lastUsedProcess string
}

func NewProcessGroup(id string, config Config, proxyLogger *LogMonitor, upstreamLogger *LogMonitor) *ProcessGroup {
	groupConfig, ok := config.Groups[id]
	if !ok {
		panic("Unable to find configuration for group id: " + id)
	}

	pg := &ProcessGroup{
		id:             id,
		config:         config,
		swap:           groupConfig.Swap,
		exclusive:      groupConfig.Exclusive,
		persistent:     groupConfig.Persistent,
		proxyLogger:    proxyLogger,
		upstreamLogger: upstreamLogger,
		processes:      make(map[string]*Process),
	}

	// Create a Process for each member in the group
	for _, modelID := range groupConfig.Members {
		modelConfig, modelID, _ := pg.config.FindConfig(modelID)
		process := NewProcess(modelID, pg.config.HealthCheckTimeout, modelConfig, pg.upstreamLogger, pg.proxyLogger)
		pg.processes[modelID] = process
	}

	return pg
}

// ProxyRequest proxies a request to the specified model
func (pg *ProcessGroup) ProxyRequest(modelID string, writer http.ResponseWriter, request *http.Request) error {
	if !pg.HasMember(modelID) {
		return fmt.Errorf("model %s not part of group %s", modelID, pg.id)
	}

	// Check if the process exists and is not nil
	process := pg.processes[modelID]
	if process == nil {
		return fmt.Errorf("process for model %s is not initialized in group %s", modelID, pg.id)
	}

	if pg.swap {
		pg.Lock()
		if pg.lastUsedProcess != modelID {

			// is there something already running?
			if pg.lastUsedProcess != "" && pg.processes[pg.lastUsedProcess] != nil {
				pg.processes[pg.lastUsedProcess].Stop()
			}

			// wait for the request to the new model to be fully handled
			// and prevent race conditions see issue #277
			process.ProxyRequest(writer, request)
			pg.lastUsedProcess = modelID

			// short circuit and exit
			pg.Unlock()
			return nil
		}
		pg.Unlock()
	}

	process.ProxyRequest(writer, request)
	return nil
}

func (pg *ProcessGroup) HasMember(modelName string) bool {
	// First check the config for members
	if groupConfig, exists := pg.config.Groups[pg.id]; exists {
		for _, member := range groupConfig.Members {
			if member == modelName {
				return true
			}
		}
	}

	// Also check the actual processes map in case it was added dynamically
	pg.Lock()
	_, hasProcess := pg.processes[modelName]
	pg.Unlock()

	return hasProcess
}

func (pg *ProcessGroup) StopProcesses(strategy StopStrategy) {
	pg.Lock()
	defer pg.Unlock()

	if len(pg.processes) == 0 {
		return
	}

	// stop Processes in parallel
	var wg sync.WaitGroup
	for _, process := range pg.processes {
		wg.Add(1)
		go func(process *Process) {
			defer wg.Done()
			switch strategy {
			case StopImmediately:
				process.StopImmediately()
			default:
				process.Stop()
			}
		}(process)
	}
	wg.Wait()
}

func (pg *ProcessGroup) Shutdown() {
	var wg sync.WaitGroup
	for _, process := range pg.processes {
		wg.Add(1)
		go func(process *Process) {
			defer wg.Done()
			process.Shutdown()
		}(process)
	}
	wg.Wait()
}
