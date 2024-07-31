package models

import "sort"

// Process 定义进程结构体
type Process struct {
	ID         uint   `json:"ID" gorm:"primaryKey"`
	Name       string `json:"Name"`
	Command    string `json:"Command"`
	WorkDir    string `json:"WorkDir"`
	User       string `json:"User"`
	RetryCount int    `json:"RetryCount"`
	AutoStart  bool   `json:"AutoStart"`
	LogFile    string `json:"LogFile"`
}

// RuntimeProcess 定义运行时进程结构体
type RuntimeProcess struct {
	Process
	PID        int    `json:"PID"`
	Status     string `json:"Status"`
	ManualStop bool   `json:"ManualStop"`
}

func SortRuntimeProcesses(processes []*RuntimeProcess) []*RuntimeProcess {
	sort.Slice(processes, func(i, j int) bool {
		return processes[i].Process.ID < processes[j].Process.ID
	})
	return processes
}

// var RuntimeProcesses map[uint]*RuntimeProcess

// const processesFilePath = "processes.json"

// func ReadProcessesFromJSON() error {
// 	file, err := os.ReadFile(processesFilePath)
// 	if err != nil {
// 		if os.IsNotExist(err) {
// 			// 如果文件不存在,创建一个空的JSON文件
// 			return WriteProcessesToJSON()
// 		}
// 		return err
// 	}

// 	var processes []Process
// 	if err := json.Unmarshal(file, &processes); err != nil {
// 		return err
// 	}

// 	RuntimeProcesses = make(map[uint]*RuntimeProcess)
// 	for _, p := range processes {
// 		rp := &RuntimeProcess{
// 			Process:    p,
// 			PID:        0,
// 			Status:     "stopped",
// 			ManualStop: false,
// 		}
// 		RuntimeProcesses[p.ID] = rp
// 	}

// 	return nil
// }

// func WriteProcessesToJSON() error {
// 	var processes []Process
// 	for _, rp := range RuntimeProcesses {
// 		processes = append(processes, rp.Process)
// 	}

// 	data, err := json.MarshalIndent(processes, "", "  ")
// 	if err != nil {
// 		return err
// 	}

// 	return os.WriteFile(processesFilePath, data, 0644)
// }
