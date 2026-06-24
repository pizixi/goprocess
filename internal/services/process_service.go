package services

import (
	"errors"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/natefinch/lumberjack"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/websocket"
	"github.com/pizixi/goprocess/pkg/utils"
)

const (
	retryBudgetResetAfter = time.Minute
	stopTimeout           = 10 * time.Second
)

type ProcessService struct {
	PM *models.ProcessManager
}

func NewProcessService(pm *models.ProcessManager) *ProcessService {
	return &ProcessService{PM: pm}
}

func (ps *ProcessService) InitializeAndAutoStartProcesses() {
	time.Sleep(2 * time.Second)
	for _, rp := range ps.PM.GetAllProcesses() {
		if rp.AutoStart {
			log.Printf("Starting process: %d \n", rp.ID)
			if err := ps.StartProcessAsync(rp.ID); err != nil {
				log.Printf("Error auto-starting process %d: %v", rp.ID, err)
			}
		}
	}
}

func (ps *ProcessService) SetupCloseHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutting down...")
		ps.StopAllProcesses()
		os.Exit(0)
	}()
}

func (ps *ProcessService) StopAllProcesses() {
	var wg sync.WaitGroup
	for _, rp := range ps.PM.GetAllProcesses() {
		if models.IsActiveStatus(rp.Status) {
			wg.Add(1)
			go func(id uint) {
				defer wg.Done()
				ps.StopProcessByID(id)
			}(rp.ID)
		}
	}
	wg.Wait()
	log.Println("All processes stopped")
}

func (ps *ProcessService) RestartProcessByID(id uint) error {
	rp, exists := ps.PM.GetSnapshot(id)
	if !exists {
		return errors.New("process not found")
	}

	switch rp.Status {
	case "starting", "stopping":
		return fmt.Errorf("process %d is %s", rp.ID, rp.Status)
	case "running", "retrying":
		stopSnapshot, shouldStop, err := ps.prepareStop(id)
		if err != nil {
			return err
		}
		if !shouldStop {
			return fmt.Errorf("process %d is %s", rp.ID, rp.Status)
		}
		go func() {
			ps.stopPreparedProcess(id, stopSnapshot)
			if _, exists := ps.PM.SetManualStop(id, false); !exists {
				return
			}
			ps.StartProcessById(id)
		}()
	default:
		if _, exists := ps.PM.SetManualStop(id, false); !exists {
			return errors.New("process not found")
		}
		if err := ps.StartProcessAsync(id); err != nil {
			return err
		}
	}

	return nil
}

func (ps *ProcessService) StartProcessAsync(id uint) error {
	rp, runID, ok := ps.PM.BeginStart(id)
	if !ok {
		if snapshot, exists := ps.PM.GetSnapshot(id); exists {
			if snapshot.ManualStop {
				return fmt.Errorf("process %d was manually stopped", id)
			}
			return fmt.Errorf("process %d is %s", id, snapshot.Status)
		}
		return errors.New("process not found")
	}

	websocket.BroadcastStatus(rp)
	go ps.runProcess(id, runID, rp)
	return nil
}

func (ps *ProcessService) StartProcessById(id uint) {
	rp, runID, ok := ps.PM.BeginStart(id)
	if !ok {
		if snapshot, exists := ps.PM.GetSnapshot(id); exists {
			if snapshot.ManualStop {
				log.Printf("Process %d was manually stopped. Not restarting.", id)
				return
			}
			log.Printf("Process %d is %s. Not starting a duplicate worker.", id, snapshot.Status)
			return
		}
		log.Printf("Error fetching process %d", id)
		return
	}

	websocket.BroadcastStatus(rp)
	ps.runProcess(id, runID, rp)
}

func (ps *ProcessService) runProcess(id uint, runID uint64, initial models.RuntimeProcess) {
	logDir := filepath.Join("logs", fmt.Sprintf("process_%d", initial.ID))
	if err := os.MkdirAll(logDir, 0755); err != nil {
		log.Printf("Error creating log directory for process %d: %v", id, err)
		if snapshot, ok := ps.PM.FinishRunStatus(id, runID, "error", 0); ok {
			websocket.BroadcastStatus(snapshot)
		}
		return
	}

	logFile := &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "output.log"),
		MaxSize:    100,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}
	defer logFile.Close()

	retryCount := 0

retryLoop:
	for {
		rp, current := ps.PM.GetRunSnapshot(id, runID)
		if !current {
			return
		}
		if rp.ManualStop {
			ps.finishManualRun(id, runID)
			return
		}
		if retryCount >= rp.RetryCount {
			break
		}

		cmd := kexec.CommandString(rp.Command)
		cmd.Dir = rp.WorkDir

		var cleanupProcessUser func()
		var err error
		var lastTokenWaitLog time.Time
		tokenWaitStarted := time.Now()
		for {
			latestProcess, current := ps.PM.GetRunSnapshot(id, runID)
			if !current {
				return
			}
			if latestProcess.ManualStop {
				log.Printf("Process %d was manually stopped while waiting for user %q token", id, rp.User)
				ps.finishManualRun(id, runID)
				return
			}

			cleanupProcessUser, err = utils.ConfigureProcessUser(cmd.Cmd, rp.User)
			if err == nil {
				break
			}
			if !utils.IsProcessUserTokenUnavailable(err) {
				log.Printf("Error configuring user %q for process %d: %v", rp.User, id, err)
				retryCount++
				if retryCount >= rp.RetryCount || !ps.markRetryingAndWait(id, runID, time.Duration(retryCount)*time.Second) {
					break retryLoop
				}
				continue retryLoop
			}

			if lastTokenWaitLog.IsZero() || time.Since(lastTokenWaitLog) >= 30*time.Second {
				log.Printf("Waiting for user %q token before starting process %d, elapsed %s", rp.User, id, time.Since(tokenWaitStarted).Round(time.Second))
				lastTokenWaitLog = time.Now()
			}
			if !ps.waitOrFinishManualStop(id, runID, 5*time.Second) {
				return
			}
		}

		latestProcess, current := ps.PM.GetRunSnapshot(id, runID)
		if !current {
			if cleanupProcessUser != nil {
				cleanupProcessUser()
			}
			return
		}
		if latestProcess.ManualStop {
			if cleanupProcessUser != nil {
				cleanupProcessUser()
			}
			ps.finishManualRun(id, runID)
			return
		}

		cmd.Stdout = logFile
		cmd.Stderr = logFile
		ps.PM.SetLogFile(id, logFile.Filename)

		if err := cmd.Start(); err != nil {
			if cleanupProcessUser != nil {
				cleanupProcessUser()
			}
			log.Printf("Error starting process %d: %v", id, err)
			retryCount++
			if retryCount >= rp.RetryCount || !ps.markRetryingAndWait(id, runID, time.Duration(retryCount)*time.Second) {
				break
			}
			continue
		}
		if cleanupProcessUser != nil {
			cleanupProcessUser()
		}

		if !ps.PM.SetCommandIfCurrent(id, runID, cmd) {
			_ = cmd.Terminate(os.Kill)
			_ = cmd.Wait()
			return
		}

		if snapshot, ok := ps.PM.UpdateProcessStatusIfCurrent(id, runID, "running", cmd.Process.Pid); ok {
			websocket.BroadcastStatus(snapshot)
		} else {
			_ = cmd.Terminate(os.Kill)
			_ = cmd.Wait()
			return
		}

		runStarted := time.Now()
		_ = cmd.Wait()
		ps.PM.RemoveCommandIf(id, cmd)

		latestProcess, current = ps.PM.GetRunSnapshot(id, runID)
		if !current {
			return
		}
		if latestProcess.ManualStop {
			log.Printf("Process %d was manually stopped. Not restarting.", id)
			ps.finishManualRun(id, runID)
			return
		}

		log.Println(id, "触发自动重启")
		if time.Since(runStarted) > retryBudgetResetAfter {
			retryCount = 0
		}
		retryCount++

		latestProcess, current = ps.PM.GetRunSnapshot(id, runID)
		if !current {
			return
		}
		if retryCount >= latestProcess.RetryCount {
			break
		}
		if !ps.markRetryingAndWait(id, runID, time.Duration(retryCount+1)*time.Second) {
			return
		}
	}

	latestProcess, current := ps.PM.GetRunSnapshot(id, runID)
	if !current {
		return
	}
	if latestProcess.ManualStop {
		ps.finishManualRun(id, runID)
		return
	}
	if retryCount >= latestProcess.RetryCount {
		if snapshot, ok := ps.PM.FinishRunStatus(id, runID, "error", 0); ok {
			websocket.BroadcastStatus(snapshot)
		}
	} else if snapshot, ok := ps.PM.FinishRunStatus(id, runID, "stopped", 0); ok {
		websocket.BroadcastStatus(snapshot)
	}
}

func (ps *ProcessService) finishManualRun(id uint, runID uint64) {
	if snapshot, ok := ps.PM.FinishRunStatus(id, runID, "stopped", 0); ok {
		websocket.BroadcastStatus(snapshot)
	}
}

func (ps *ProcessService) markRetryingAndWait(id uint, runID uint64, delay time.Duration) bool {
	snapshot, current := ps.PM.GetRunSnapshot(id, runID)
	if !current {
		return false
	}
	if snapshot.ManualStop {
		ps.finishManualRun(id, runID)
		return false
	}
	if snapshot, ok := ps.PM.UpdateProcessStatusIfCurrent(id, runID, "retrying", 0); ok {
		websocket.BroadcastStatus(snapshot)
	} else {
		return false
	}
	return ps.waitOrFinishManualStop(id, runID, delay)
}

func (ps *ProcessService) waitOrFinishManualStop(id uint, runID uint64, delay time.Duration) bool {
	deadline := time.Now().Add(delay)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return true
		}
		if remaining > time.Second {
			remaining = time.Second
		}
		time.Sleep(remaining)

		snapshot, current := ps.PM.GetRunSnapshot(id, runID)
		if !current {
			return false
		}
		if snapshot.ManualStop {
			ps.finishManualRun(id, runID)
			return false
		}
	}
}

func (ps *ProcessService) StopProcessByID(id uint) {
	rp, shouldStop, err := ps.prepareStop(id)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	if !shouldStop {
		return
	}
	ps.stopPreparedProcess(id, rp)
}

func (ps *ProcessService) StopProcessAsync(id uint) error {
	rp, shouldStop, err := ps.prepareStop(id)
	if err != nil {
		return err
	}
	if !shouldStop {
		return nil
	}
	go ps.stopPreparedProcess(id, rp)
	return nil
}

func (ps *ProcessService) prepareStop(id uint) (models.RuntimeProcess, bool, error) {
	rp, rpExists := ps.PM.GetSnapshot(id)
	if !rpExists {
		return models.RuntimeProcess{}, false, fmt.Errorf("process %d not found", id)
	}
	if rp.Status == "stopped" || rp.Status == "error" || rp.Status == "stopping" {
		return rp, false, nil
	}

	log.Printf("Stopping process %s (ID: %d, PID: %d)", rp.Name, id, rp.PID)

	ps.PM.SetManualStop(id, true)
	if snapshot, ok := ps.PM.UpdateProcessStatus(id, "stopping", rp.PID); ok {
		websocket.BroadcastStatus(snapshot)
		return snapshot, true, nil
	}
	return models.RuntimeProcess{}, false, fmt.Errorf("process %d not found", id)
}

func (ps *ProcessService) stopPreparedProcess(id uint, rp models.RuntimeProcess) {
	cmd, exists := ps.PM.GetCommand(id)
	if !exists {
		log.Printf("Process %d has no active command", id)
		if snapshot, ok := ps.PM.MarkStoppedIfManual(id); ok {
			websocket.BroadcastStatus(snapshot)
		}
		return
	}

	stopch := make(chan error, 1)
	go func() {
		stopch <- cmd.Terminate(syscall.SIGTERM)
	}()

	select {
	case err := <-stopch:
		if err != nil {
			log.Printf("Error sending stop signal to process %d: %v", id, err)
		}
		log.Printf("Process %s (ID: %d, PID: %d) received stop signal", rp.Name, id, rp.PID)
	case <-time.After(stopTimeout):
		log.Printf("Process %s (ID: %d, PID: %d) stop timed out, force killing", rp.Name, id, rp.PID)
		if err := cmd.Terminate(os.Kill); err != nil {
			log.Printf("Error force killing process %d: %v", id, err)
		}
	}

	err := cmd.Wait()

	ps.PM.RemoveCommandIf(id, cmd)

	if snapshot, ok := ps.PM.MarkStoppedIfManual(id); ok {
		websocket.BroadcastStatus(snapshot)
	}

	if err != nil && err.Error() != "signal: killed" {
		log.Printf("Error waiting for process %d to stop: %v", id, err)
	} else {
		log.Printf("Process %s (ID: %d) has been fully stopped", rp.Name, id)
	}
}
