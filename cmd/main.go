package goprocess

import (
	"context"
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"path/filepath"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/natefinch/lumberjack"
	"github.com/pizixi/goprocess/internal/config"
	"github.com/pizixi/goprocess/internal/handlers"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/services"
	"github.com/pizixi/goprocess/internal/websocket"
	"github.com/pizixi/goprocess/web"
	"github.com/robfig/cron/v3"
)

var PS *services.ProcessService

func GoprocessMain() {
	// 读取配置文件
	if err := config.ReadConfigFromJSON(); err != nil {
		panic("failed to read config from JSON: " + err.Error())
	}

	db, err := models.NewGormDatabase("processes.db")
	if err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	logFile := &lumberjack.Logger{
		Filename:   filepath.Join("logs", "goprocess.log"),
		MaxSize:    10,
		MaxBackups: 3,
		MaxAge:     28,
		Compress:   true,
	}

	// 重定向标准输出到日志文件
	log.SetOutput(logFile)

	pm := models.NewProcessManager(db)
	PS = services.NewProcessService(pm)

	processHandler := handlers.NewHandler(pm, PS)
	if err := pm.LoadProcesses(context.Background()); err != nil {
		log.Fatalf("加载进程失败: %v", err)
	}

	cronJob := cron.New()
	taskManager := models.NewTaskManager(db, cronJob)
	taskService := services.NewTaskService(cronJob)

	// 加载所有任务
	if err := taskManager.LoadTasks(context.Background()); err != nil {
		log.Fatalf("加载任务失败: %v", err)
	}
	// 初始化并启动已启用的任务
	enabledTasks := taskManager.GetEnabledTasks()
	log.Printf("找到 %d 个已启用的任务", len(enabledTasks))
	for _, task := range enabledTasks {
		taskCopy := task // 创建一个副本以避免闭包问题
		taskService.ScheduleTask(&taskCopy)
	}

	taskHandler := handlers.NewTaskHandler(taskManager, taskService)

	// 初始化Echo框架
	e := echo.New()

	// 自定义日志格式
	loggerConfig := middleware.LoggerConfig{
		Format: "${time_rfc3339_nano}  ${method}  ${uri}  ${status}\n",
		// Output: file,
		Output: logFile,
	}

	// 使用自定义配置的日志中间件
	e.Use(middleware.LoggerWithConfig(loggerConfig))
	e.Use(middleware.Recover())

	// 渲染嵌入的HTML文件
	renderer := &TemplateRenderer{
		templates: template.Must(template.New("").ParseFS(web.ViewsFS, "views/*")),
	}
	e.Renderer = renderer
	e.Debug = true

	// 设置静态文件服务
	staticRootFS, _ := fs.Sub(web.StaticFS, "static")
	httpFS := http.FS(staticRootFS)
	e.StaticFS("/static", httpFSAdapter{httpFS})

	// 设置路由
	// setupRoutes(e)
	e.GET("/login", handlers.LoginHandler)
	e.POST("/login", handlers.LoginPostHandler)
	e.GET("/logout", handlers.LogoutHandler)

	if config.Conf.HTTPAuth.Enabled {
		// 受保护的路由
		e.GET("/", handlers.HomeHandler, handlers.AuthMiddleware)
		// 进程管理
		e.GET("/processes.html", handlers.ProcessesHandler, handlers.AuthMiddleware)
		e.GET("/api/processes", processHandler.ListProcessesHandler, handlers.AuthMiddleware)
		e.GET("/api/processes/:id", processHandler.GetProcessHandler, handlers.AuthMiddleware)
		e.POST("/api/process", processHandler.CreateProcessHandler, handlers.AuthMiddleware)
		e.PUT("/api/process/:id", processHandler.UpdateProcessHandler, handlers.AuthMiddleware)
		e.DELETE("/api/process/:id", processHandler.DeleteProcessHandler, handlers.AuthMiddleware)
		e.POST("/api/process/:id/start", processHandler.StartProcessHandler, handlers.AuthMiddleware)
		e.POST("/api/process/:id/stop", processHandler.StopProcessHandler, handlers.AuthMiddleware)
		e.GET("/api/process/:id/logstream", processHandler.GetProcesseLogsHandler, handlers.AuthMiddleware)
		e.GET("/ws", websocket.HandleWebSocket, handlers.AuthMiddleware)

		// 定时任务
		e.GET("/tasks.html", handlers.TasksHandler, handlers.AuthMiddleware)
		e.GET("/api/tasks", taskHandler.ListTasksHandler, handlers.AuthMiddleware)
		e.POST("/api/tasks", taskHandler.CreateTaskHandler, handlers.AuthMiddleware)
		e.PUT("/api/tasks/:id", taskHandler.UpdateTaskHandler, handlers.AuthMiddleware)
		e.DELETE("/api/tasks/:id", taskHandler.DeleteTaskHandler, handlers.AuthMiddleware)
		e.POST("/api/tasks/:id/toggle", taskHandler.ToggleTaskStatusHandler, handlers.AuthMiddleware)
		e.POST("/api/tasks/:id/run", taskHandler.RunTaskHandler, handlers.AuthMiddleware)
		e.GET("/api/tasks/:id/logs", taskHandler.GetTaskLogsHandler, handlers.AuthMiddleware)

		// 系统日志
		e.GET("/serverlogs.html", handlers.ServerLogsHandler, handlers.AuthMiddleware)
		e.GET("/api/serverlogs", handlers.GetServerLogsHandler, handlers.AuthMiddleware)
	} else {
		// 不受保护的路由
		e.GET("/", handlers.HomeHandler)
		// 进程管理
		e.GET("/processes.html", handlers.ProcessesHandler)
		e.GET("/api/processes", processHandler.ListProcessesHandler)
		e.GET("/api/processes/:id", processHandler.GetProcessHandler)
		e.POST("/api/process", processHandler.CreateProcessHandler)
		e.PUT("/api/process/:id", processHandler.UpdateProcessHandler)
		e.DELETE("/api/process/:id", processHandler.DeleteProcessHandler)
		e.POST("/api/process/:id/start", processHandler.StartProcessHandler)
		e.POST("/api/process/:id/stop", processHandler.StopProcessHandler)
		e.GET("/api/process/:id/logstream", processHandler.GetProcesseLogsHandler)
		e.GET("/ws", websocket.HandleWebSocket)

		// 定时任务
		e.GET("/tasks.html", handlers.TasksHandler)
		e.GET("/api/tasks", taskHandler.ListTasksHandler)
		e.POST("/api/tasks", taskHandler.CreateTaskHandler)
		e.PUT("/api/tasks/:id", taskHandler.UpdateTaskHandler)
		e.DELETE("/api/tasks/:id", taskHandler.DeleteTaskHandler)
		e.POST("/api/tasks/:id/toggle", taskHandler.ToggleTaskStatusHandler)
		e.POST("/api/tasks/:id/run", taskHandler.RunTaskHandler)
		e.GET("/api/tasks/:id/logs", taskHandler.GetTaskLogsHandler)

		// 系统日志
		e.GET("/serverlogs.html", handlers.ServerLogsHandler)
		e.GET("/api/serverlogs", handlers.GetServerLogsHandler)
	}

	// 自动启动进程
	go PS.InitializeAndAutoStartProcesses()

	// 启动定时任务
	cronJob.Start()
	// 定期打印已启用的任务（每30秒打印一次）
	// go func() {
	// 	ticker := time.NewTicker(30 * time.Second)
	// 	for range ticker.C {
	// 		taskService.PrintEnabledTasks()
	// 	}
	// }()

	defer cronJob.Stop()

	// 设置关闭进程处理
	PS.SetupCloseHandler()

	// 启动服务器
	e.Logger.Fatal(e.Start(config.Conf.Addr))
}

// TemplateRenderer 是自定义的模板渲染器
type TemplateRenderer struct {
	templates *template.Template
}

// Render 渲染模板
func (t *TemplateRenderer) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return t.templates.ExecuteTemplate(w, name, data)
}

// 创建一个适配器，将 http.FileSystem 转换为 fs.FS
type httpFSAdapter struct {
	httpFS http.FileSystem
}

func (h httpFSAdapter) Open(name string) (fs.File, error) {
	return h.httpFS.Open(name)
}
