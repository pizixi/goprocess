package goprocess

import (
	"html/template"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"

	"github.com/codeskyblue/kexec"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/pizixi/goprocess/internal/config"
	"github.com/pizixi/goprocess/internal/handlers"
	"github.com/pizixi/goprocess/internal/models"
	"github.com/pizixi/goprocess/internal/services"
	"github.com/pizixi/goprocess/internal/websocket"
	"github.com/pizixi/goprocess/web"
)

func GoprocessMain() {
	// 读取配置文件
	if err := config.ReadConfigFromJSON(); err != nil {
		panic("failed to read config from JSON: " + err.Error())
	}

	// 初始化进程数据
	if err := models.InitDB(); err != nil {
		panic("failed to InitDB: " + err.Error())
	}
	// // 初始化进程数据
	// if err := models.ReadProcessesFromJSON(); err != nil {
	// 	panic("failed to read processes from JSON: " + err.Error())
	// }

	// // 初始化定时任务数据
	// if err := models.ReadTasksFromJSON(); err != nil {
	// 	panic("failed to read tasks from JSON: " + err.Error())
	// }

	services.Processes = make(map[uint]*kexec.KCommand)

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

	// 自定义日志格式
	loggerConfig := middleware.LoggerConfig{
		Format: "${time_rfc3339_nano}  ${method}  ${uri}  ${status}\n",
		Output: file,
	}

	// 使用自定义配置的日志中间件
	e.Use(middleware.LoggerWithConfig(loggerConfig))
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

	// 设置路由
	setupRoutes(e)

	// 自动启动进程
	go services.InitializeAndAutoStartProcesses()

	// 启动定时任务
	go services.InitializeCronJob()

	// 设置关闭处理
	services.SetupCloseHandler()

	// 程序退出时停止 CronJob
	services.CronJobCleanup()

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

func setupRoutes(e *echo.Echo) {
	e.GET("/login", handlers.LoginHandler)
	e.POST("/login", handlers.LoginPostHandler)
	e.GET("/logout", handlers.LogoutHandler)

	if config.Conf.HTTPAuth.Enabled {
		// 受保护的路由
		e.GET("/", handlers.HomeHandler, handlers.AuthMiddleware)
		e.GET("/processes.html", handlers.ProcessesHandler, handlers.AuthMiddleware)
		e.GET("/processes", handlers.ListProcessesHandler, handlers.AuthMiddleware)
		e.GET("/processes/:id", handlers.GetProcessHandler, handlers.AuthMiddleware)
		e.POST("/process", handlers.CreateProcessHandler, handlers.AuthMiddleware)
		e.PUT("/process/:id", handlers.UpdateProcessHandler, handlers.AuthMiddleware)
		e.DELETE("/process/:id", handlers.DeleteProcessHandler, handlers.AuthMiddleware)
		e.POST("/process/:id/start", handlers.StartProcessHandler, handlers.AuthMiddleware)
		e.POST("/process/:id/stop", handlers.StopProcessHandler, handlers.AuthMiddleware)
		e.GET("/process/:id/logstream", handlers.LogStreamHandler, handlers.AuthMiddleware)
		e.GET("/ws", websocket.HandleWebSocket, handlers.AuthMiddleware)

		e.GET("/tasks.html", handlers.TasksHandler, handlers.AuthMiddleware)
		e.GET("/api/tasks", handlers.GetTasksHandler, handlers.AuthMiddleware)
		e.POST("/api/tasks", handlers.CreateTaskHandler, handlers.AuthMiddleware)
		e.PUT("/api/tasks/:id", handlers.UpdateTaskHandler, handlers.AuthMiddleware)
		e.DELETE("/api/tasks/:id", handlers.DeleteTaskHandler, handlers.AuthMiddleware)
		e.POST("/api/tasks/:id/toggle", handlers.ToggleTaskHandler, handlers.AuthMiddleware)
		e.POST("/api/tasks/:id/run", handlers.RunTaskHandler, handlers.AuthMiddleware)
		e.GET("/api/tasks/:id/logs", handlers.GetTaskLogsHandler, handlers.AuthMiddleware)
	} else {
		// 不受保护的路由
		e.GET("/", handlers.HomeHandler)
		e.GET("/processes.html", handlers.ProcessesHandler)
		e.GET("/processes", handlers.ListProcessesHandler)
		e.GET("/processes/:id", handlers.GetProcessHandler)
		e.POST("/process", handlers.CreateProcessHandler)
		e.PUT("/process/:id", handlers.UpdateProcessHandler)
		e.DELETE("/process/:id", handlers.DeleteProcessHandler)
		e.POST("/process/:id/start", handlers.StartProcessHandler)
		e.POST("/process/:id/stop", handlers.StopProcessHandler)
		e.GET("/process/:id/logstream", handlers.LogStreamHandler)
		e.GET("/ws", websocket.HandleWebSocket)

		e.GET("/tasks.html", handlers.TasksHandler)
		e.GET("/api/tasks", handlers.GetTasksHandler)
		e.POST("/api/tasks", handlers.CreateTaskHandler)
		e.PUT("/api/tasks/:id", handlers.UpdateTaskHandler)
		e.DELETE("/api/tasks/:id", handlers.DeleteTaskHandler)
		e.POST("/api/tasks/:id/toggle", handlers.ToggleTaskHandler)
		e.POST("/api/tasks/:id/run", handlers.RunTaskHandler)
		e.GET("/api/tasks/:id/logs", handlers.GetTaskLogsHandler)
	}
}
