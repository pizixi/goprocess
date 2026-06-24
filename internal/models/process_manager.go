package models

import (
	"context"
	"sync"

	"github.com/codeskyblue/kexec"
)

type ProcessManager struct {
	processes map[uint]*RuntimeProcess
	commands  map[uint]*kexec.KCommand
	runIDs    map[uint]uint64
	nextRunID uint64
	mu        sync.RWMutex
	db        Database
}

func NewProcessManager(db Database) *ProcessManager {
	pm := &ProcessManager{
		processes: make(map[uint]*RuntimeProcess),
		commands:  make(map[uint]*kexec.KCommand),
		runIDs:    make(map[uint]uint64),
		db:        db,
	}
	// pm.loadProcesses()
	return pm
}

func (pm *ProcessManager) LoadProcesses(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	var processes []Process
	err := pm.db.Find(ctx, &processes)
	if err != nil {
		return err
	}

	for _, p := range processes {
		rp := &RuntimeProcess{
			Process:    p,
			PID:        0,
			Status:     "stopped",
			ManualStop: false,
		}
		pm.processes[p.ID] = rp
	}
	return nil
}

func (pm *ProcessManager) GetAllProcesses() []*RuntimeProcess {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	processes := make([]*RuntimeProcess, 0, len(pm.processes))
	for _, rp := range pm.processes {
		snapshot := *rp
		processes = append(processes, &snapshot)
	}
	return processes
}

func (pm *ProcessManager) GetProcess(id uint) (*RuntimeProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rp, exists := pm.processes[id]
	return rp, exists
}

func (pm *ProcessManager) GetSnapshot(id uint) (RuntimeProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rp, exists := pm.processes[id]
	if !exists {
		return RuntimeProcess{}, false
	}
	return *rp, true
}

func (pm *ProcessManager) AddProcess(ctx context.Context, p *Process) (*RuntimeProcess, error) {
	// pm.mu.RLock()
	// defer pm.mu.RUnlock()
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if err := pm.db.Create(ctx, p); err != nil {
		return nil, err
	}

	rp := &RuntimeProcess{
		Process:    *p,
		PID:        0,
		Status:     "stopped",
		ManualStop: false,
	}

	// pm.mu.Lock()
	pm.processes[p.ID] = rp
	// pm.mu.Unlock()

	snapshot := *rp
	return &snapshot, nil
}

func (pm *ProcessManager) UpdateProcess(ctx context.Context, p *Process) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if rp, exists := pm.processes[p.ID]; exists {
		rp.Process = *p
		return pm.db.Save(ctx, p)
	}
	return nil
}

func (pm *ProcessManager) DeleteProcess(ctx context.Context, id uint) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if rp, exists := pm.processes[id]; exists {
		delete(pm.processes, id)
		delete(pm.commands, id)
		delete(pm.runIDs, id)
		return pm.db.Delete(ctx, rp.Process)
	}
	return nil
}

func (pm *ProcessManager) BeginStart(id uint) (RuntimeProcess, uint64, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rp, exists := pm.processes[id]
	if !exists || rp.ManualStop || IsActiveStatus(rp.Status) {
		if exists {
			return *rp, 0, false
		}
		return RuntimeProcess{}, 0, false
	}

	pm.nextRunID++
	runID := pm.nextRunID
	pm.runIDs[id] = runID
	rp.Status = "starting"
	rp.PID = 0
	return *rp, runID, true
}

func (pm *ProcessManager) GetRunSnapshot(id uint, runID uint64) (RuntimeProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rp, exists := pm.processes[id]
	if !exists || pm.runIDs[id] != runID {
		return RuntimeProcess{}, false
	}
	return *rp, true
}

func (pm *ProcessManager) UpdateProcessStatusIfCurrent(id uint, runID uint64, status string, pid int) (RuntimeProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rp, exists := pm.processes[id]
	if !exists || pm.runIDs[id] != runID {
		return RuntimeProcess{}, false
	}
	rp.Status = status
	rp.PID = pid
	return *rp, true
}

func (pm *ProcessManager) FinishRunStatus(id uint, runID uint64, status string, pid int) (RuntimeProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rp, exists := pm.processes[id]
	if !exists || pm.runIDs[id] != runID {
		return RuntimeProcess{}, false
	}
	rp.Status = status
	rp.PID = pid
	delete(pm.runIDs, id)
	return *rp, true
}

func (pm *ProcessManager) SetCommandIfCurrent(id uint, runID uint64, cmd *kexec.KCommand) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if _, exists := pm.processes[id]; !exists || pm.runIDs[id] != runID {
		return false
	}
	pm.commands[id] = cmd
	return true
}

func (pm *ProcessManager) GetCommand(id uint) (*kexec.KCommand, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	cmd, exists := pm.commands[id]
	return cmd, exists
}

func (pm *ProcessManager) RemoveCommandIf(id uint, cmd *kexec.KCommand) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	if pm.commands[id] != cmd {
		return false
	}
	delete(pm.commands, id)
	return true
}

func (pm *ProcessManager) UpdateProcessStatus(id uint, status string, pid int) (RuntimeProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if rp, exists := pm.processes[id]; exists {
		rp.Status = status
		rp.PID = pid
		return *rp, true
	}
	return RuntimeProcess{}, false
}

func (pm *ProcessManager) MarkStoppedIfManual(id uint) (RuntimeProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	rp, exists := pm.processes[id]
	if !exists || !rp.ManualStop {
		return RuntimeProcess{}, false
	}
	rp.Status = "stopped"
	rp.PID = 0
	return *rp, true
}

func (pm *ProcessManager) SetManualStop(id uint, manualStop bool) (RuntimeProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if rp, exists := pm.processes[id]; exists {
		rp.ManualStop = manualStop
		return *rp, true
	}
	return RuntimeProcess{}, false
}

func (pm *ProcessManager) SetLogFile(id uint, logFile string) (RuntimeProcess, bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if rp, exists := pm.processes[id]; exists {
		rp.LogFile = logFile
		return *rp, true
	}
	return RuntimeProcess{}, false
}

func IsActiveStatus(status string) bool {
	switch status {
	case "starting", "running", "retrying", "stopping":
		return true
	default:
		return false
	}
}
