package models

import (
	"context"
	"sync"

	"github.com/codeskyblue/kexec"
)

type ProcessManager struct {
	processes map[uint]*RuntimeProcess
	commands  map[uint]*kexec.KCommand
	mu        sync.RWMutex
	db        Database
}

func NewProcessManager(db Database) *ProcessManager {
	pm := &ProcessManager{
		processes: make(map[uint]*RuntimeProcess),
		commands:  make(map[uint]*kexec.KCommand),
		db:        db,
	}
	// pm.loadProcesses()
	return pm
}

func (pm *ProcessManager) LoadProcesses(ctx context.Context) error {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
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
		processes = append(processes, rp)
	}
	return processes
}

func (pm *ProcessManager) GetProcess(id uint) (*RuntimeProcess, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	rp, exists := pm.processes[id]
	return rp, exists
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

	return rp, nil
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
		return pm.db.Delete(ctx, rp.Process)
	}
	return nil
}

func (pm *ProcessManager) SetCommand(id uint, cmd *kexec.KCommand) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	pm.commands[id] = cmd
}

func (pm *ProcessManager) GetCommand(id uint) (*kexec.KCommand, bool) {
	pm.mu.RLock()
	defer pm.mu.RUnlock()
	cmd, exists := pm.commands[id]
	return cmd, exists
}

func (pm *ProcessManager) RemoveCommand(id uint) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.commands, id)
}

func (pm *ProcessManager) UpdateProcessStatus(id uint, status string, pid int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if rp, exists := pm.processes[id]; exists {
		rp.Status = status
		rp.PID = pid
	}
}

func (pm *ProcessManager) SetManualStop(id uint, manualStop bool) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if rp, exists := pm.processes[id]; exists {
		rp.ManualStop = manualStop
	}
}
