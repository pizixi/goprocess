package handlers

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"
)

// FileInfo 文件信息结构体
type FileInfo struct {
	Name        string `json:"name"`
	Size        int64  `json:"size"`
	IsDirectory bool   `json:"isDirectory"`
	UpdatedAt   string `json:"updatedAt"`
}

// Config 配置信息
type Config struct {
	RootPath string // 文件存储根路径
}

var fileManagerConfig Config

func init() {
	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}

	fileManagerConfig = Config{
		RootPath: currentDir,
	}
}

// ListFiles 列出指定路径下的所有文件和目录
func ListFiles(c echo.Context) error {
	requestPath := c.QueryParam("path")
	if requestPath == "" {
		requestPath = "/"
	}
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	fullPath := filepath.Join(fileManagerConfig.RootPath, filepath.Clean(requestPath))

	if !strings.HasPrefix(filepath.Clean(fullPath), fileManagerConfig.RootPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	files := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil || strings.HasPrefix(info.Name(), ".") {
			continue
		}

		files = append(files, FileInfo{
			Name:        info.Name(),
			Size:        info.Size(),
			IsDirectory: info.IsDir(),
			UpdatedAt:   info.ModTime().Format("2006-01-02 15:04:05"),
		})
	}

	return c.JSON(http.StatusOK, files)
}

// UploadFile 处理文件上传
func UploadFile(c echo.Context) error {
	uploadPath := c.FormValue("path")
	if !strings.HasPrefix(uploadPath, "/") {
		uploadPath = "/" + uploadPath
	}

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	fullPath := filepath.Join(fileManagerConfig.RootPath, filepath.Clean(uploadPath), file.Filename)

	if !strings.HasPrefix(filepath.Clean(fullPath), fileManagerConfig.RootPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	src, err := file.Open()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer src.Close()

	dst, err := os.Create(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}
	defer dst.Close()

	if _, err = io.Copy(dst, src); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "文件上传成功",
	})
}

// DownloadFile 处理文件下载
func DownloadFile(c echo.Context) error {
	requestPath := c.QueryParam("path")
	if !strings.HasPrefix(requestPath, "/") {
		requestPath = "/" + requestPath
	}

	fullPath := filepath.Join(fileManagerConfig.RootPath, filepath.Clean(requestPath))

	if !strings.HasPrefix(filepath.Clean(fullPath), fileManagerConfig.RootPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, "File not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if info.IsDir() {
		return echo.NewHTTPError(http.StatusBadRequest, "Cannot download a directory")
	}

	return c.Attachment(fullPath, filepath.Base(fullPath))
}

// DeleteFileOrFolder 处理文件或文件夹的删除
func DeleteFileOrFolder(c echo.Context) error {
	path := c.QueryParam("path")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	fullPath := filepath.Join(fileManagerConfig.RootPath, filepath.Clean(path))

	if !strings.HasPrefix(filepath.Clean(fullPath), fileManagerConfig.RootPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	err := os.RemoveAll(fullPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "文件或文件夹已成功删除",
	})
}

// CreateFolder 处理新建文件夹
func CreateFolder(c echo.Context) error {
	path := c.FormValue("path")
	folderName := c.FormValue("name")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	fullPath := filepath.Join(fileManagerConfig.RootPath, filepath.Clean(path), folderName)

	if !strings.HasPrefix(filepath.Clean(fullPath), fileManagerConfig.RootPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	err := os.MkdirAll(fullPath, 0755)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "文件夹创建成功",
	})
}
