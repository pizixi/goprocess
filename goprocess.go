package main

import (
	"bufio"
	"encoding/json"
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
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/nxadm/tail"
	"github.com/pizixi/goprocess/web"

	"github.com/codeskyblue/kexec"
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
)

// 定义Process结构体
type Process struct {
	ID         uint   `json:"ID"`
	Name       string `json:"Name"`
	Command    string `json:"Command"`
	WorkDir    string `json:"WorkDir"`
	User       string `json:"User"`
	RetryCount int    `json:"RetryCount"`
	AutoStart  bool   `json:"AutoStart"`
	LogFile    string `json:"LogFile"`
}

// 定义RuntimeProcess结构体
type RuntimeProcess struct {
	Process
	PID        int    `json:"PID"`
	Status     string `json:"Status"`
	ManualStop bool   `json:"ManualStop"`
}

type Config struct {
	HTTPAuth struct {
		Enabled  bool   `json:"enabled"`
		Username string `json:"username"`
		Password string `json:"password"`
	} `json:"httpauth"`
	Addr string `json:"addr"`
}

// 全局变量
var config Config
var processes map[uint]*kexec.KCommand
var runtimeProcesses map[uint]*RuntimeProcess
var mu sync.Mutex
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

var sessions = make(map[string]string)

// JSON文件路径
const processesFilePath = "processes.json"
const configFilePath = "./goprocess.json"

// 读取进程数据从JSON文件
func readProcessesFromJSON() error {
	file, err := os.ReadFile(processesFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果文件不存在,创建一个空的JSON文件
			return writeProcessesToJSON()
		}
		return err
	}

	var processes []Process
	if err := json.Unmarshal(file, &processes); err != nil {
		return err
	}

	runtimeProcesses = make(map[uint]*RuntimeProcess)
	for _, p := range processes {
		rp := &RuntimeProcess{
			Process:    p,
			PID:        0,
			Status:     "stopped",
			ManualStop: false,
		}
		runtimeProcesses[p.ID] = rp
	}

	return nil
}

// 写入进程数据到JSON文件
func writeProcessesToJSON() error {
	var processes []Process
	for _, rp := range runtimeProcesses {
		processes = append(processes, rp.Process)
	}

	data, err := json.MarshalIndent(processes, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(processesFilePath, data, 0644)
}

func readConfigFromJSON() error {
	// 尝试读取配置文件
	file, err := os.ReadFile(configFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			// 如果配置文件不存在, 创建一个带有默认值的配置文件
			defaultConfig := Config{
				HTTPAuth: struct {
					Enabled  bool   `json:"enabled"`
					Username string `json:"username"`
					Password string `json:"password"`
				}{Enabled: false},
				Addr: "127.0.0.1:11315",
			}
			config = defaultConfig
			return writeConfigToJSON()
		}
		return err
	}

	// 解析配置文件
	if err := json.Unmarshal(file, &config); err != nil {
		return err
	}

	return nil
}

func writeConfigToJSON() error {
	// 序列化配置并写入文件
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(configFilePath, data, 0644)
}

// 认证中间件
func authMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessions[sessionID.Value] == "" {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		return next(c)
	}
}

// 创建一个适配器，将 http.FileSystem 转换为 fs.FS
type httpFSAdapter struct {
	httpFS http.FileSystem
}

func (h httpFSAdapter) Open(name string) (fs.File, error) {
	return h.httpFS.Open(name)
}

func GoprocessMain() {
	// 读取配置文件
	if err := readConfigFromJSON(); err != nil {
		panic("failed to read config from JSON: " + err.Error())
	}

	// 初始化进程数据
	if err := readProcessesFromJSON(); err != nil {
		panic("failed to read processes from JSON: " + err.Error())
	}

	processes = make(map[uint]*kexec.KCommand)

	// 打开日志文件
	file, err := os.OpenFile("goprocess.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("Failed to open goprocess.log: %v", err)
	}
	defer file.Close()
	// 重定向标准输出到日志文件
	log.SetOutput(file)

	// 初始化Echo框架
	e := echo.New()

	// 设置中间件
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())

	// 渲染嵌入的HTML文件
	renderer := &TemplateRenderer{
		templates: template.Must(template.New("").ParseFS(web.ViewsFS, "views/*")),
	}
	e.Renderer = renderer

	// 设置静态文件服务
	staticRootFS, _ := fs.Sub(web.StaticFS, "static")
	httpFS := http.FS(staticRootFS)
	e.StaticFS("/static", httpFSAdapter{httpFS})

	// 路由设置
	e.GET("/login", loginHandler)
	e.POST("/login", loginPostHandler)
	e.GET("/logout", logoutHandler)

	if config.HTTPAuth.Enabled {
		// 受保护的路由
		e.GET("/", homeHandler, authMiddleware)
		e.GET("/processes", listProcessesHandler, authMiddleware)
		e.GET("/processes/:id", getProcessHandler, authMiddleware)
		e.POST("/process", createProcessHandler, authMiddleware)
		e.PUT("/process/:id", updateProcessHandler, authMiddleware)
		e.DELETE("/process/:id", deleteProcessHandler, authMiddleware)
		e.POST("/process/:id/start", startProcessHandler, authMiddleware)
		e.POST("/process/:id/stop", stopProcessHandler, authMiddleware)
		e.GET("/process/:id/logstream", logStreamHandler, authMiddleware)
		e.GET("/ws", handleWebSocket, authMiddleware)
	} else {
		// 不受保护的路由
		e.GET("/", homeHandler)
		e.GET("/processes", listProcessesHandler)
		e.GET("/processes/:id", getProcessHandler)
		e.POST("/process", createProcessHandler)
		e.PUT("/process/:id", updateProcessHandler)
		e.DELETE("/process/:id", deleteProcessHandler)
		e.POST("/process/:id/start", startProcessHandler)
		e.POST("/process/:id/stop", stopProcessHandler)
		e.GET("/process/:id/logstream", logStreamHandler)
		e.GET("/ws", handleWebSocket)
	}

	// 自动启动进程
	go initializeAndAutoStartProcesses()

	// 设置关闭处理
	setupCloseHandler()

	// 启动服务器
	e.Logger.Fatal(e.Start(config.Addr))
}

// TemplateRenderer 是自定义的模板渲染器
type TemplateRenderer struct {
	templates *template.Template
}

// Render 渲染模板
func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// 处理函数...

func loginHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "login.html", nil)
}

func loginPostHandler(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username == config.HTTPAuth.Username && password == config.HTTPAuth.Password {
		sessionID := uuid.New().String()
		sessions[sessionID] = username
		cookie := new(http.Cookie)
		cookie.Name = "session_id"
		cookie.Value = sessionID
		cookie.Expires = time.Now().Add(8 * time.Hour)
		c.SetCookie(cookie)
		return c.Redirect(http.StatusSeeOther, "/")
	}
	return c.Render(http.StatusUnauthorized, "login.html", map[string]interface{}{"error": "Invalid username or password"})
}

func logoutHandler(c echo.Context) error {
	cookie, _ := c.Cookie("session_id")
	if cookie != nil {
		delete(sessions, cookie.Value)
	}
	c.SetCookie(&http.Cookie{
		Name:    "session_id",
		Value:   "",
		Expires: time.Now().Add(-1 * time.Hour),
	})
	return c.Redirect(http.StatusSeeOther, "/login")
}

func homeHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "index.html", nil)
}

func listProcessesHandler(c echo.Context) error {
	var rps []RuntimeProcess
	for _, rp := range runtimeProcesses {
		rps = append(rps, *rp)
	}
	sort.Slice(rps, func(i, j int) bool {
		return rps[i].ID < rps[j].ID
	})
	return c.JSON(http.StatusOK, rps)
}

func getProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}
	return c.JSON(http.StatusOK, rp)
}

func createProcessHandler(c echo.Context) error {
	var p Process
	if err := c.Bind(&p); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if p.RetryCount == 0 {
		p.RetryCount = 3
	}
	p.ID = uint(len(runtimeProcesses) + 1)
	rp := &RuntimeProcess{
		Process:    p,
		PID:        0,
		Status:     "stopped",
		ManualStop: false,
	}
	runtimeProcesses[p.ID] = rp
	if err := writeProcessesToJSON(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to save process"})
	}
	return c.JSON(http.StatusOK, rp)
}

func updateProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}
	if err := c.Bind(&rp.Process); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": err.Error()})
	}
	if err := writeProcessesToJSON(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to update process"})
	}
	return c.JSON(http.StatusOK, rp)
}

func deleteProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status == "running" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "Process is still running. Please stop it first."})
	}

	delete(runtimeProcesses, uint(id))
	if err := writeProcessesToJSON(); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "Failed to delete process"})
	}
	return c.JSON(http.StatusOK, map[string]string{"message": "Process deleted"})
}

func startProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status != "stopped" && rp.Status != "error" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("Process %d is %s", rp.ID, rp.Status)})
	}

	rp.ManualStop = false
	go startProcessById(uint(id))

	return c.JSON(http.StatusOK, map[string]string{"status": "starting", "message": "Process is being started"})
}

func stopProcessHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	if rp.Status == "stopped" {
		return c.JSON(http.StatusOK, map[string]string{"status": "stopped", "message": "Process already stopped"})
	}

	rp.Status = "stopping"
	rp.ManualStop = true
	broadcastStatus(*rp)

	go stopProcessByID(uint(id))

	return c.JSON(http.StatusOK, map[string]string{"status": "stopping", "message": "Process is being stopped"})
}

func logStreamHandler(c echo.Context) error {
	id, _ := strconv.Atoi(c.Param("id"))
	rp, exists := runtimeProcesses[uint(id)]
	if !exists {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "Process not found"})
	}

	c.Response().Header().Set("Content-Type", "text/event-stream")
	c.Response().Header().Set("Cache-Control", "no-cache")
	c.Response().Header().Set("Connection", "keep-alive")
	c.Response().Header().Set("Access-Control-Allow-Origin", "*")

	file, err := os.Open(rp.LogFile)
	if err != nil {
		log.Printf("无法打开文件: %v\n", err)
		return err
	}
	defer file.Close()

	fileInfo, err := file.Stat()
	if err != nil {
		log.Printf("获取文件信息失败: %v\n", err)
		return err
	}
	byteCount := fileInfo.Size()
	log.Printf("文件 '%s' 的字节数: %d\n", rp.LogFile, byteCount)

	offset := calculateOffset(file, byteCount, 10000)
	log.Println("offset:", offset)

	if rp.LogFile == "" {
		return c.String(http.StatusInternalServerError, "data: Log file not specified\n\n")
	}

	tailFile, err := tail.TailFile(rp.LogFile, tail.Config{
		ReOpen:    true,
		Follow:    true,
		Location:  &tail.SeekInfo{Offset: offset, Whence: 2},
		MustExist: false,
		Poll:      true,
	})

	if err != nil {
		return c.String(http.StatusInternalServerError, fmt.Sprintf("data: Failed to tail log file: %s\n\n", err.Error()))
	}
	defer tailFile.Cleanup()

	c.Response().Writer.WriteHeader(http.StatusOK)

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-c.Request().Context().Done():
				return
			case msg, ok := <-tailFile.Lines:
				if !ok {
					c.String(http.StatusInternalServerError, "data: Tail file closed unexpectedly\n\n")
					return
				}
				if msg.Err != nil {
					c.String(http.StatusInternalServerError, fmt.Sprintf("data: Error reading log file: %s\n\n", msg.Err.Error()))
					return
				}
				line := strings.TrimRight(EnsureUTF8(msg.Text), "\r\n")
				line = strings.TrimRight(EnsureUTF8(line), "\n")
				if line != "" {
					c.String(http.StatusOK, fmt.Sprintf("data: %s\n\n", line))
					c.Response().Flush()
				}
			}
		}
	}()

	<-c.Request().Context().Done()
	<-done
	return nil
}

func handleWebSocket(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		return err
	}
	defer ws.Close()

	clients[ws] = true

	for {
		_, _, err := ws.ReadMessage()
		if err != nil {
			log.Println(err)
			delete(clients, ws)
			return nil
		}
	}
}

// 辅助函数

func initializeAndAutoStartProcesses() {
	time.Sleep(2 * time.Second)
	for _, rp := range runtimeProcesses {
		if rp.AutoStart {
			log.Printf("Starting process: %d \n", rp.ID)
			go startProcessById(rp.ID)
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

func EnsureUTF8(data string) string {
	if utf8.ValidString(data) {
		return data
	}
	utf8Data, _, err := transform.String(simplifiedchinese.GBK.NewDecoder(), data)
	if err == nil {
		return utf8Data
	}
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
var broadcast = make(chan RuntimeProcess)

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
