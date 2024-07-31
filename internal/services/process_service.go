package services

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/natefinch/lumberjack"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/websocket"
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
			go ps.StartProcessById(rp.ID)
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
	for _, rp := range ps.PM.GetAllProcesses() {
		if rp.Status == "running" || rp.Status == "starting" {
			ps.StopProcessByID(rp.ID)
		}
	}
	log.Println("All processes stopped")
}

func (ps *ProcessService) StartProcessById(id uint) {
	rp, exists := ps.PM.GetProcess(id)
	if !exists {
		log.Printf("Error fetching process %d", id)
		return
	}

	if rp.ManualStop {
		log.Printf("Process %d was manually stopped. Not restarting.", id)
		return
	}

	ps.PM.UpdateProcessStatus(id, "starting", 0)
	websocket.BroadcastStatus(*rp)

	startTime := time.Now()
	retryCount := 0
	for retryCount < rp.RetryCount {
		cmd := kexec.CommandString(rp.Command)
		cmd.Dir = rp.WorkDir

		logDir := filepath.Join("logs", fmt.Sprintf("process_%d", rp.ID))
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Printf("Error creating log directory for process %d: %v", id, err)
			ps.PM.UpdateProcessStatus(id, "error", 0)
			websocket.BroadcastStatus(*rp)
			return
		}

		logFile := &lumberjack.Logger{
			Filename:   filepath.Join(logDir, "output.log"),
			MaxSize:    10,
			MaxBackups: 3,
			MaxAge:     28,
			Compress:   true,
		}

		rp.LogFile = logFile.Filename
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		ps.PM.SetCommand(rp.ID, cmd)

		if err := cmd.Start(); err != nil {
			log.Printf("Error starting process %d: %v", id, err)
			retryCount++
			time.Sleep(time.Duration(retryCount) * time.Second)
			continue
		}

		ps.PM.UpdateProcessStatus(id, "running", cmd.Process.Pid)
		if time.Since(startTime) > time.Minute {
			retryCount = 0
		}
		go func() {
			time.Sleep(1 * time.Second)
			websocket.BroadcastStatus(*rp)
		}()

		cmd.Wait()

		ps.PM.RemoveCommand(rp.ID)

		if rp.ManualStop {
			log.Printf("Process %d was manually stopped. Not restarting.", id)
			break
		}

		log.Println(rp.ID, "触发自动重启")

		retryCount++
		time.Sleep(time.Duration(retryCount+1) * time.Second)
	}

	if retryCount >= rp.RetryCount {
		ps.PM.UpdateProcessStatus(id, "error", 0)
	} else {
		ps.PM.UpdateProcessStatus(id, "stopped", 0)
	}
	go func() {
		time.Sleep(1 * time.Second)
		websocket.BroadcastStatus(*rp)
	}()
}

func (ps *ProcessService) StopProcessByID(id uint) {
	cmd, exists := ps.PM.GetCommand(id)
	rp, rpExists := ps.PM.GetProcess(id)

	if !exists || !rpExists {
		log.Printf("Process %d not found", id)
		return
	}

	log.Printf("Stopping process %s (ID: %d, PID: %d)", rp.Name, id, rp.PID)

	ps.PM.UpdateProcessStatus(id, "stopping", rp.PID)
	ps.PM.SetManualStop(id, true)

	stopch := make(chan bool)
	go func() {
		cmd.Terminate(syscall.SIGTERM)
		stopch <- true
	}()

	select {
	case <-stopch:
		log.Printf("Process %s (ID: %d, PID: %d) received stop signal", rp.Name, id, rp.PID)
	case <-time.After(10 * time.Second):
		log.Printf("Process %s (ID: %d, PID: %d) stop timed out, force killing", rp.Name, id, rp.PID)
		if runtime.GOOS == "windows" {
			cmd.Terminate(os.Kill)
		} else {
			cmd.Process.Signal(syscall.SIGKILL)
		}
	}

	err := cmd.Wait()

	ps.PM.RemoveCommand(id)

	ps.PM.UpdateProcessStatus(id, "stopped", 0)

	if err != nil && err.Error() != "signal: killed" {
		log.Printf("Error waiting for process %d to stop: %v", id, err)
	} else {
		log.Printf("Process %s (ID: %d) has been fully stopped", rp.Name, id)
	}
}
