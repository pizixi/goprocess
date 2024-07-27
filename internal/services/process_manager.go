package services

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/codeskyblue/kexec"
	"github.com/natefinch/lumberjack"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/websocket"
)

var Processes map[uint]*kexec.KCommand
var mu sync.Mutex

func InitializeAndAutoStartProcesses() {
	time.Sleep(2 * time.Second)
	for _, rp := range models.RuntimeProcesses {
		if rp.AutoStart {
			log.Printf("Starting process: %d \n", rp.ID)
			go StartProcessById(rp.ID)
		}
	}
}

func SetupCloseHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutting down...")
		StopAllProcesses()
		os.Exit(0)
	}()
}

func StopAllProcesses() {
	var wg sync.WaitGroup
	for id, rp := range models.RuntimeProcesses {
		if rp.Status == "running" || rp.Status == "starting" {
			wg.Add(1)
			go func(id uint) {
				defer wg.Done()
				StopProcessByID(id)
			}(id)
		}
	}
	wg.Wait()
	log.Println("All processes stopped")
}

func StartProcessById(id uint) {
	rp, exists := models.RuntimeProcesses[id]
	if !exists {
		log.Printf("Error fetching process %d", id)
		return
	}

	if rp.ManualStop {
		log.Printf("Process %d was manually stopped. Not restarting.", id)
		return
	}

	rp.Status = "starting"
	websocket.BroadcastStatus(*rp)

	startTime := time.Now()
	retryCount := 0
	for retryCount < rp.RetryCount {
		cmd := kexec.CommandString(rp.Command)
		cmd.Dir = rp.WorkDir

		logDir := filepath.Join("logs", fmt.Sprintf("process_%d", rp.ID))
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Printf("Error creating log directory for process %d: %v", id, err)
			rp.Status = "error"
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

		mu.Lock()
		Processes[rp.ID] = cmd
		mu.Unlock()

		if err := cmd.Start(); err != nil {
			log.Printf("Error starting process %d: %v", id, err)
			retryCount++
			time.Sleep(time.Duration(retryCount) * time.Second)
			continue
		}

		rp.PID = cmd.Process.Pid
		rp.Status = "running"
		if time.Since(startTime) > time.Minute {
			retryCount = 0
		}
		go func() {
			time.Sleep(1 * time.Second)
			websocket.BroadcastStatus(*rp)
		}()

		cmd.Wait()

		mu.Lock()
		delete(Processes, rp.ID)
		mu.Unlock()

		if rp.ManualStop {
			log.Printf("Process %d was manually stopped. Not restarting.", id)
			break
		}

		log.Println(rp.ID, "触发自动重启")

		retryCount++
		time.Sleep(time.Duration(retryCount+1) * time.Second)
	}

	if retryCount >= rp.RetryCount {
		rp.Status = "error"
	} else {
		rp.Status = "stopped"
	}
	rp.PID = 0
	go func() {
		time.Sleep(1 * time.Second)
		websocket.BroadcastStatus(*rp)
	}()
}

func StopProcessByID(id uint) {
	mu.Lock()
	cmd, exists := Processes[id]
	rp, rpExists := models.RuntimeProcesses[id]
	mu.Unlock()

	if !exists || !rpExists {
		log.Printf("Process %d not found", id)
		return
	}

	log.Printf("Stopping process %s (ID: %d, PID: %d)", rp.Name, id, rp.PID)

	rp.Status = "stopping"
	rp.ManualStop = true
	// websocket.BroadcastStatus(*rp)

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

	mu.Lock()
	delete(Processes, id)
	mu.Unlock()

	rp.Status = "stopped"
	rp.PID = 0

	if err != nil && err.Error() != "signal: killed" {
		log.Printf("Error waiting for process %d to stop: %v", id, err)
	} else {
		log.Printf("Process %s (ID: %d) has been fully stopped", rp.Name, id)
	}
}
