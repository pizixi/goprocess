package main

import (
	"bufio"
	"fmt"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/nxadm/tail"
	"github.com/pizixi/goprocess/web"

	"github.com/codeskyblue/kexec"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/gorilla/websocket"
	"github.com/natefinch/lumberjack"
	"golang.org/x/text/encoding"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/japanese"
	"golang.org/x/text/encoding/korean"
	"golang.org/x/text/encoding/simplifiedchinese"
	"golang.org/x/text/encoding/traditionalchinese"
	"golang.org/x/text/encoding/unicode"
	"golang.org/x/text/transform"
	"gorm.io/gorm"
)

type Process struct {
	ID         uint `gorm:"primaryKey"`
	Name       string
	Command    string
	WorkDir    string
	User       string
	RetryCount int
	AutoStart  bool
	LogFile    string
	// ManualStop bool
}

type RuntimeProcess struct {
	Process
	PID        int
	Status     string
	ManualStop bool // 将 ManualStop 移动到 RuntimeProcess 结构体中
}

var db *gorm.DB
var processes map[uint]*kexec.KCommand
var runtimeProcesses map[uint]*RuntimeProcess
var mu sync.Mutex
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var sessions = make(map[string]string)

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessions[sessionID] == "" {
			c.Redirect(http.StatusSeeOther, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func GoprocessMain() {
	var err error
	db, err = gorm.Open(sqlite.Open("processes.db"), &gorm.Config{})
	if err != nil {
		panic("failed to connect database")
	}
	db.AutoMigrate(&Process{})

	processes = make(map[uint]*kexec.KCommand)
	runtimeProcesses = make(map[uint]*RuntimeProcess)

	// 打开日志文件
	file, err := os.OpenFile("goprocess.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open gin.log: %v", err)
	}
	defer file.Close()
	// 重定向标准输出到日志文件
	log.SetOutput(file)
	// 设置 Gin 的日志输出为文件和控制台
	gin.DefaultWriter = io.MultiWriter(file, os.Stdout)

	r := gin.Default()

	// 不使用embed
	// r.LoadHTMLFiles("index.html")

	// 渲染嵌入的 HTML 文件
	tmpl := template.Must(template.New("").ParseFS(web.ViewsFS, "views/*"))
	r.SetHTMLTemplate(tmpl)
	// 设置静态文件服务
	staticRootFS, _ := fs.Sub(web.StaticFS, "static")
	r.StaticFS("/static", http.FS(staticRootFS))

	// r.GET("/", func(c *gin.Context) {
	// 	c.HTML(200, "index.html", nil)
	// })

	// r.GET("/processes", listProcesses)
	// r.GET("/processes/:id", getProcess)
	// r.POST("/process", createProcess)
	// r.PUT("/process/:id", updateProcess)
	// r.DELETE("/process/:id", deleteProcess)
	// r.POST("/process/:id/start", startProcess)
	// r.POST("/process/:id/stop", stopProcess)
	// r.GET("/process/:id/logstream", logStream)
	// r.GET("/ws", handleWebSocket)

	r.GET("/login", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", gin.H{})
	})

	r.POST("/login", func(c *gin.Context) {
		username := c.PostForm("username")
		password := c.PostForm("password")

		// 这里应该进行实际的用户验证
		if username == "admin" && password == "admin999" {
			sessionID := uuid.New().String()
			sessions[sessionID] = username
			c.SetCookie("session_id", sessionID, 8*3600, "/", "", false, true)
			c.Redirect(http.StatusSeeOther, "/")
		} else {
			c.HTML(http.StatusUnauthorized, "login.html", gin.H{"error": "Invalid username or password"})
		}
	})

	r.GET("/logout", func(c *gin.Context) {
		sessionID, _ := c.Cookie("session_id")
		delete(sessions, sessionID)
		c.SetCookie("session_id", "", -1, "/", "", false, true)
		c.Redirect(http.StatusSeeOther, "/login")
	})

	r.GET("/", authMiddleware(), func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// 为需要鉴权的路由添加 authMiddleware
	authorized := r.Group("/", authMiddleware())
	{
		authorized.GET("/processes", listProcesses)
		authorized.GET("/processes/:id", getProcess)
		authorized.POST("/process", createProcess)
		authorized.PUT("/process/:id", updateProcess)
		authorized.DELETE("/process/:id", deleteProcess)
		authorized.POST("/process/:id/start", startProcess)
		authorized.POST("/process/:id/stop", stopProcess)
		authorized.GET("/process/:id/logstream", logStream)
		authorized.GET("/ws", handleWebSocket)
	}

	// 自动启动进程
	go initializeAndAutoStartProcesses()

	// 设置关闭处理
	setupCloseHandler()

	r.Run(":11315")
}

func initializeAndAutoStartProcesses() {
	time.Sleep(2 * time.Second)
	var ps []Process
	db.Find(&ps)

	for _, p := range ps {
		rp := &RuntimeProcess{
			Process:    p,
			PID:        0,
			Status:     "stopped",
			ManualStop: false, // 初始化时设置为非手动停止状态
		}
		runtimeProcesses[p.ID] = rp
		if p.AutoStart {
			log.Printf("Starting process: %d \n", p.ID)
			go startProcessById(p.ID)
		}
	}
}

func setupCloseHandler() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Println("Shutting down...")
		stopAllProcesses()
		os.Exit(0)
	}()
}

func stopAllProcesses() {
	var wg sync.WaitGroup
	for id, rp := range runtimeProcesses {
		if rp.Status == "running" || rp.Status == "starting" {
			wg.Add(1)
			go func(id uint) {
				defer wg.Done()
				stopProcessByID(id)
			}(id)
		}
	}
	wg.Wait()
	log.Println("All processes stopped")
}
func listProcesses(c *gin.Context) {
	var rps []RuntimeProcess
	for _, rp := range runtimeProcesses {
		rps = append(rps, *rp)
	}
	// 根据 ID 升序排序
	sort.Slice(rps, func(i, j int) bool {
		return rps[i].ID < rps[j].ID
	})

	c.JSON(http.StatusOK, rps)
}

func getProcess(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}
	c.JSON(http.StatusOK, rp)
}

func createProcess(c *gin.Context) {
	var p Process
	if err := c.ShouldBindJSON(&p); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if p.RetryCount == 0 {
		p.RetryCount = 3
	}
	db.Create(&p)
	rp := &RuntimeProcess{
		Process:    p,
		PID:        0,
		Status:     "stopped",
		ManualStop: false, // 创建新进程时设置为非手动停止状态
	}
	runtimeProcesses[p.ID] = rp
	c.JSON(http.StatusOK, rp)
}
func updateProcess(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}
	if err := c.ShouldBindJSON(&rp.Process); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	db.Save(&rp.Process)
	c.JSON(http.StatusOK, rp)
}

func deleteProcess(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}

	if rp.Status == "running" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Process is still running. Please stop it first."})
		return
	}

	db.Delete(&rp.Process)
	delete(runtimeProcesses, uint(id))
	c.JSON(http.StatusOK, gin.H{"message": "Process deleted"})
}

func startProcess(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}

	if rp.Status != "stopped" && rp.Status != "error" {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Process %d is %s", rp.ID, rp.Status)})
		return
	}

	rp.ManualStop = false
	// db.Save(&rp.Process) // 移除对数据库的保存操作，因为 ManualStop 不再持久化

	c.JSON(http.StatusOK, gin.H{"status": "starting", "message": "Process is being started"})

	go startProcessById(uint(id))
}

func startProcessById(id uint) {
	rp, exists := runtimeProcesses[id]
	if !exists {
		log.Printf("Error fetching process %d", id)
		return
	}

	if rp.ManualStop {
		log.Printf("Process %d was manually stopped. Not restarting.", id)
		return
	}

	rp.Status = "starting"
	broadcastStatus(*rp)

	startTime := time.Now()
	retryCount := 0
	for retryCount < rp.RetryCount {
		cmd := kexec.CommandString(rp.Command)
		cmd.Dir = rp.WorkDir

		logDir := filepath.Join("logs", fmt.Sprintf("process_%d", rp.ID))
		if err := os.MkdirAll(logDir, 0755); err != nil {
			log.Printf("Error creating log directory for process %d: %v", id, err)
			rp.Status = "error"
			broadcastStatus(*rp)
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
		processes[rp.ID] = cmd
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
		broadcastStatus(*rp)

		cmd.Wait()

		mu.Lock()
		delete(processes, rp.ID)
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
	broadcastStatus(*rp)
}

func stopProcess(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}

	if rp.Status == "stopped" {
		c.JSON(http.StatusOK, gin.H{"status": "stopped", "message": "Process already stopped"})
		return
	}

	rp.Status = "stopping"
	rp.ManualStop = true
	broadcastStatus(*rp)

	c.JSON(http.StatusOK, gin.H{"status": "stopping", "message": "Process is being stopped"})

	go stopProcessByID(uint(id))
}

func stopProcessByID(id uint) {
	mu.Lock()
	cmd, exists := processes[id]
	rp, rpExists := runtimeProcesses[id]
	mu.Unlock()

	if !exists || !rpExists {
		log.Printf("Process %d not found", id)
		return
	}

	log.Printf("Stopping process %s (ID: %d, PID: %d)", rp.Name, id, rp.PID)

	rp.Status = "stopping"
	rp.ManualStop = true
	broadcastStatus(*rp)

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
	delete(processes, id)
	mu.Unlock()

	rp.Status = "stopped"
	rp.PID = 0
	broadcastStatus(*rp)

	if err != nil && err.Error() != "signal: killed" {
		log.Printf("Error waiting for process %d to stop: %v", id, err)
	} else {
		log.Printf("Process %s (ID: %d) has been fully stopped", rp.Name, id)
	}
}

func logStream(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "Process not found"})
		return
	}

	c.Header("Content-Type", "text/event-stream; charset=utf-8")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("Access-Control-Allow-Origin", "*")

	file, err := os.Open(rp.LogFile)
	if err != nil {
		log.Printf("无法打开文件: %v\n", err)
		return
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("获取文件信息失败: %v\n", err)
		return
	}
	byteCount := fileInfo.Size()
	log.Printf("文件 '%s' 的字节数: %d\n", rp.LogFile, byteCount)

	// 展示最近10000行左右的日志
	offset := calculateOffset(file, byteCount, 10000)
	log.Println("offset:", offset)

	if rp.LogFile == "" {
		c.SSEvent("error", "Log file not specified")
		return
	}

	tailFile, err := tail.TailFile(rp.LogFile, tail.Config{
		ReOpen:    true,
		Follow:    true,
		Location:  &tail.SeekInfo{Offset: offset, Whence: 2},
		MustExist: false,
		Poll:      true,
	})

	if err != nil {
		c.SSEvent("error", "Failed to tail log file: "+err.Error())
		return
	}
	defer tailFile.Cleanup()

	ctx := c.Request.Context()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			select {
			case <-ctx.Done():
				return
			case msg, ok := <-tailFile.Lines:
				if !ok {
					c.SSEvent("error", "Tail file closed unexpectedly")
					return
				}
				if msg.Err != nil {
					c.SSEvent("error", "Error reading log file: "+msg.Err.Error())
					return
				}
				line := strings.TrimRight(EnsureUTF8(msg.Text), "\r\n")
				line = strings.TrimRight(EnsureUTF8(line), "\n")
				if line != "" {
					c.SSEvent("message", line)
					c.Writer.Flush()
				}
			}
		}
	}()

	<-ctx.Done()
	<-done
}

// 计算偏移量
func calculateOffset(file *os.File, byteCount int64, seekCount int64) int64 {
	if byteCount <= seekCount {
		return -byteCount
	}

	_, err := file.Seek(-seekCount, io.SeekEnd)
	if err != nil {
		log.Printf("文件定位失败: %v\n", err)
		return -seekCount
	}

	reader := bufio.NewReader(file)
	var offset int64 = -seekCount
	for {
		_, err := reader.ReadByte()
		if err != nil {
			break
		}
		offset++
		if reader.Buffered() > 0 {
			nextByte, _ := reader.Peek(1)
			if nextByte[0] == '\n' {
				break
			}
		}
	}

	return offset
}

// EnsureUTF8 尝试
func EnsureUTF8(data string) string {
	if utf8.ValidString(data) {
		return data
	}
	utf8Data, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), data)
	if err == nil {
		return utf8Data
	}
	// 如果不是UTF-8编码，则尝试将其转换为UTF-8编码
	byteData := []byte(data)
	encodings := []encoding.Encoding{
		unicode.UTF8,
		unicode.UTF16(unicode.BigEndian, unicode.UseBOM),
		unicode.UTF16(unicode.LittleEndian, unicode.UseBOM),
		simplifiedchinese.GBK,
		simplifiedchinese.GB18030,
		traditionalchinese.Big5,
		japanese.ShiftJIS,
		korean.EUCKR,
		charmap.ISO8859_1,
		charmap.ISO8859_2,
		charmap.ISO8859_3,
		charmap.ISO8859_4,
		charmap.ISO8859_5,
		charmap.ISO8859_6,
		charmap.ISO8859_7,
		charmap.ISO8859_8,
		charmap.ISO8859_9,
		charmap.ISO8859_10,
		charmap.ISO8859_13,
		charmap.ISO8859_14,
		charmap.ISO8859_15,
		charmap.ISO8859_16,
		charmap.Windows1250,
		charmap.Windows1251,
		charmap.Windows1252,
		charmap.Windows1253,
		charmap.Windows1254,
		charmap.Windows1255,
		charmap.Windows1256,
		charmap.Windows1257,
		charmap.Windows1258,
		charmap.KOI8R,
		charmap.KOI8U,
	}

	for _, enc := range encodings {
		ret, err := transformString(byteData, enc, unicode.UTF8)
		if err == nil {
			return ret
		}
	}

	return ""
}
func transformString(data []byte, src, dest encoding.Encoding) (string, error) {
	transformer := transform.Chain(src.NewDecoder(), dest.NewEncoder())
	res, _, err := transform.Bytes(transformer, data)
	if err != nil {
		return "", err
	}
	return string(res), nil
}

var clients = make(map[*websocket.Conn]bool)

// var broadcast = make(chan Process)
var broadcast = make(chan RuntimeProcess)

func handleWebSocket(c *gin.Context) {
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Could not open websocket connection"})
		log.Println(err)
		return
	}
	defer conn.Close()

	clients[conn] = true

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			log.Println(err)
			delete(clients, conn)
			return
		}
	}
}

func broadcastStatus(rp RuntimeProcess) {
	broadcast <- rp
}

func init() {
	go handleMessages()
}

func handleMessages() {
	for {
		rp := <-broadcast
		for client := range clients {
			err := client.WriteJSON(rp)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(clients, client)
			}
		}
	}
}
