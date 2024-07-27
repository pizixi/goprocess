package handlers

import (
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"github.com/pizixi/goprocess/internal/config"
)

var sessions = make(map[string]string)

func LoginHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "login.html", nil)
}
func HomeHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "index.html", nil)
}
func ProcessesHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "processes.html", nil)
}
func TasksHandler(c echo.Context) error {
	return c.Render(http.StatusOK, "tasks.html", nil)
}
func LoginPostHandler(c echo.Context) error {
	username := c.FormValue("username")
	password := c.FormValue("password")

	if username == config.Conf.HTTPAuth.Username && password == config.Conf.HTTPAuth.Password {
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
func LogoutHandler(c echo.Context) error {
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

func AuthMiddleware(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		sessionID, err := c.Cookie("session_id")
		if err != nil || sessions[sessionID.Value] == "" {
			return c.Redirect(http.StatusSeeOther, "/login")
		}
		return next(c)
	}
}
