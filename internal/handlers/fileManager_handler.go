package handlers

import (
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
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
	RootPaths []string // 文件存储根路径列表
}

var fileManagerConfig Config

func init() {
	fileManagerConfig = Config{
		RootPaths: getRootPaths(),
	}
}

// ensureDir 确保目录存在
func ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// ListFiles 列出指定路径下的所有文件和目录
func ListFiles(c echo.Context) error {
	requestPath := c.QueryParam("path")
	if requestPath == "" {
		// 返回根目录列表
		return c.JSON(http.StatusOK, fileManagerConfig.RootPaths)
	}

	// 使用filepath.Clean来规范化路径，去除多余的斜杠
	requestPath = filepath.Clean(requestPath)

	fullPath := requestPath

	// // 确保路径在允许的范围内
	// if !isPathAllowed(fullPath) {
	// 	return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	// }

	// fullpath 中:.中的.改成/
	fullPath = strings.Replace(fullPath, ":.", ":/", -1)
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
	uploadPath = filepath.Clean(uploadPath) // 使用filepath.Clean规范化路径

	file, err := c.FormFile("file")
	if err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}

	fullPath := filepath.Join(uploadPath, file.Filename)

	// 检查路径是否在允许的根路径内
	if !isPathAllowed(fullPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	if err := ensureDir(filepath.Dir(fullPath)); err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
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
	requestPath = filepath.Clean(requestPath) // 使用filepath.Clean规范化路径

	// 检查路径是否在允许的根路径内
	if !isPathAllowed(requestPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	info, err := os.Stat(requestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return echo.NewHTTPError(http.StatusNotFound, "File not found")
		}
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	if info.IsDir() {
		return echo.NewHTTPError(http.StatusBadRequest, "Cannot download a directory")
	}

	return c.Attachment(requestPath, filepath.Base(requestPath))
}

// DeleteFileOrFolder 处理文件或文件夹的删除
func DeleteFileOrFolder(c echo.Context) error {
	requestPath := c.QueryParam("path")
	requestPath = filepath.Clean(requestPath) // 使用filepath.Clean规范化路径

	// 检查路径是否在允许的根路径内
	if !isPathAllowed(requestPath) {
		return echo.NewHTTPError(http.StatusForbidden, "访问被拒绝")
	}

	err := os.RemoveAll(requestPath)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
	}

	return c.JSON(http.StatusOK, map[string]string{
		"message": "文件或文件夹已成功删除",
	})
}

// CreateFolder 处理新建文件夹
func CreateFolder(c echo.Context) error {
	parentPath := c.FormValue("path")
	folderName := c.FormValue("name")
	parentPath = filepath.Clean(parentPath) // 使用filepath.Clean规范化路径

	fullPath := filepath.Join(parentPath, folderName)

	// 检查路径是否在允许的根路径内
	if !isPathAllowed(fullPath) {
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

// 新增函数：检查路径是否在允许的根路径内
func isPathAllowed(checkPath string) bool {
	checkPath = filepath.Clean(checkPath)
	for _, rootPath := range fileManagerConfig.RootPaths {
		rootPath = filepath.Clean(rootPath)
		if runtime.GOOS == "windows" {
			// 对于Windows，直接比较路径前缀
			if strings.HasPrefix(strings.ToLower(checkPath), strings.ToLower(rootPath)) {
				return true
			}
		} else {
			// 对于其他系统，使用filepath.Rel
			rel, err := filepath.Rel(rootPath, checkPath)
			if err == nil && !strings.HasPrefix(rel, "..") && rel != ".." {
				return true
			}
		}
	}
	return false
}

func getRootPaths() []string {
	if runtime.GOOS == "windows" {
		return getWindowsDrives()
	}
	// 对于Linux，返回当前用户的主目录
	currentUser, err := user.Current()
	if err != nil {
		// 如果无法获取当前用户，则使用 /home 作为默认值
		return []string{"/home"}
	}
	return []string{currentUser.HomeDir}
}

func getWindowsDrives() []string {
	drives := []string{}
	for _, drive := range "ABCDEFGHIJKLMNOPQRSTUVWXYZ" {
		f, err := os.Open(string(drive) + ":\\")
		if err == nil {
			drives = append(drives, string(drive)+":\\")
			f.Close()
		}
	}
	return drives
}
