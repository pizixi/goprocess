package handlers

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/mem"
	"github.com/shirou/gopsutil/net"
)

type SystemInfo struct {
	CPU     CPUInfo       `json:"cpu"`
	Memory  MemoryInfo    `json:"memory"`
	Disk    []DiskInfo    `json:"disk"`
	Network []NetworkInfo `json:"network"`
	Host    HostInfo      `json:"host"`
	Time    time.Time     `json:"time"`
}

type CPUInfo struct {
	UsagePercent float64 `json:"usagePercent"`
	Cores        int     `json:"cores"`
	ModelName    string  `json:"modelName"`
}

type MemoryInfo struct {
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usagePercent"`
}

type DiskInfo struct {
	Path         string  `json:"path"`
	Total        uint64  `json:"total"`
	Used         uint64  `json:"used"`
	Free         uint64  `json:"free"`
	UsagePercent float64 `json:"usagePercent"`
}

type NetworkInfo struct {
	Name      string `json:"name"`
	BytesSent uint64 `json:"bytesSent"`
	BytesRecv uint64 `json:"bytesRecv"`
}

type HostInfo struct {
	Hostname string `json:"hostname"`
	Platform string `json:"platform"`
	OS       string `json:"os"`
	Uptime   uint64 `json:"uptime"`
}

func getSystemInfo() SystemInfo {
	var info SystemInfo
	info.Time = time.Now()

	// CPU信息
	cpuPercent, _ := cpu.Percent(0, false)
	cores, _ := cpu.Counts(true)
	cpuInfo, _ := cpu.Info()
	modelName := []string{}
	if len(cpuInfo) > 0 {
		for _, info := range cpuInfo {
			// 如果info.ModelName 在modelName切片中不存在，则添加
			if !slices.Contains(modelName, info.ModelName) {
				modelName = append(modelName, info.ModelName)
			}

		}
	}
	modelNames := strings.Join(modelName, " | ")
	info.CPU = CPUInfo{
		UsagePercent: cpuPercent[0],
		Cores:        cores,
		ModelName:    modelNames,
	}

	// 内存信息
	memInfo, _ := mem.VirtualMemory()
	info.Memory = MemoryInfo{
		Total:        memInfo.Total,
		Used:         memInfo.Used,
		Free:         memInfo.Free,
		UsagePercent: memInfo.UsedPercent,
	}

	// 磁盘信息
	partitions, _ := disk.Partitions(false)
	for _, partition := range partitions {
		usage, _ := disk.Usage(partition.Mountpoint)
		info.Disk = append(info.Disk, DiskInfo{
			Path:         partition.Mountpoint,
			Total:        usage.Total,
			Used:         usage.Used,
			Free:         usage.Free,
			UsagePercent: usage.UsedPercent,
		})
	}

	// 网络信息
	interfaces, _ := net.Interfaces()
	for _, iface := range interfaces {
		ioCounters, _ := net.IOCounters(true)
		for _, io := range ioCounters {
			if io.Name == iface.Name {
				info.Network = append(info.Network, NetworkInfo{
					Name:      iface.Name,
					BytesSent: io.BytesSent,
					BytesRecv: io.BytesRecv,
				})
				break
			}
		}
	}

	// 主机信息
	hostInfo, _ := host.Info()
	info.Host = HostInfo{
		Hostname: hostInfo.Hostname,
		Platform: hostInfo.Platform,
		OS:       hostInfo.OS,
		Uptime:   hostInfo.Uptime,
	}

	return info
}

func GetSystemInfoHandler(c echo.Context) error {
	info := getSystemInfo()
	return c.JSON(http.StatusOK, info)
}
