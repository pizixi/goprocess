//go:build windows

// sys_windows.go
package utils

import (
	"errors"
	"fmt"
	"os/exec"
	osuser "os/user"
	"sort"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// SetSysProcAttr sets the SysProcAttr to hide the window
func SetSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
}

// ConfigureProcessUser configures cmd to run as the requested logged-on Windows user.
// The requested user may be "user", "DOMAIN\\user", "DOMAIN/user", or ".\\user".
func ConfigureProcessUser(cmd *exec.Cmd, username string) (func(), error) {
	SetSysProcAttr(cmd)

	username = strings.TrimSpace(username)
	if username == "" || currentWindowsUserMatches(username) {
		return nil, nil
	}

	token, err := findLoggedOnUserToken(username)
	if err != nil {
		return nil, err
	}

	env, err := token.Environ(false)
	if err != nil {
		token.Close()
		return nil, fmt.Errorf("create environment for user %q: %w", username, err)
	}

	cmd.Env = env
	cmd.SysProcAttr.Token = syscall.Token(token)

	return func() {
		_ = token.Close()
	}, nil
}

func currentWindowsUserMatches(username string) bool {
	current, err := osuser.Current()
	if err != nil {
		return false
	}
	domain, name := splitWindowsAccount(current.Username)
	return matchWindowsUser(username, domain, name)
}

func findLoggedOnUserToken(username string) (windows.Token, error) {
	sessions, err := enumerateWTSSessions()
	if err != nil {
		return 0, fmt.Errorf("enumerate Windows sessions: %w", err)
	}

	var lastErr error
	for _, session := range sessions {
		token, err := duplicateUserTokenFromSession(session.SessionID)
		if err != nil {
			lastErr = err
			continue
		}

		domain, name, err := tokenAccountName(token)
		if err != nil {
			_ = token.Close()
			lastErr = err
			continue
		}

		if matchWindowsUser(username, domain, name) {
			return token, nil
		}

		_ = token.Close()
	}

	if lastErr != nil {
		if errors.Is(lastErr, windows.ERROR_ACCESS_DENIED) || errors.Is(lastErr, windows.ERROR_PRIVILEGE_NOT_HELD) {
			return 0, fmt.Errorf("query logged-on session token for user %q: %w", username, lastErr)
		}
		return 0, fmt.Errorf("%w: no logged-on session for user %q: %v", ErrProcessUserTokenUnavailable, username, lastErr)
	}
	return 0, fmt.Errorf("%w: no logged-on session for user %q", ErrProcessUserTokenUnavailable, username)
}

func enumerateWTSSessions() ([]windows.WTS_SESSION_INFO, error) {
	var sessions *windows.WTS_SESSION_INFO
	var count uint32
	if err := windows.WTSEnumerateSessions(0, 0, 1, &sessions, &count); err != nil {
		return nil, err
	}
	if sessions == nil || count == 0 {
		return nil, nil
	}
	defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(sessions)))

	result := make([]windows.WTS_SESSION_INFO, count)
	copy(result, unsafe.Slice(sessions, count))
	sort.SliceStable(result, func(i, j int) bool {
		return wtsSessionRank(result[i].State) < wtsSessionRank(result[j].State)
	})
	return result, nil
}

func wtsSessionRank(state uint32) int {
	switch state {
	case windows.WTSActive:
		return 0
	case windows.WTSConnected:
		return 1
	case windows.WTSDisconnected:
		return 2
	default:
		return 3
	}
}

func duplicateUserTokenFromSession(sessionID uint32) (windows.Token, error) {
	var impersonationToken windows.Token
	if err := windows.WTSQueryUserToken(sessionID, &impersonationToken); err != nil {
		return 0, err
	}
	defer impersonationToken.Close()

	var userToken windows.Token
	if err := windows.DuplicateTokenEx(
		impersonationToken,
		windows.TOKEN_ALL_ACCESS,
		nil,
		windows.SecurityImpersonation,
		windows.TokenPrimary,
		&userToken,
	); err != nil {
		return 0, err
	}

	return userToken, nil
}

func tokenAccountName(token windows.Token) (domain string, name string, err error) {
	tokenUser, err := token.GetTokenUser()
	if err != nil {
		return "", "", err
	}

	var nameLen uint32
	var domainLen uint32
	var use uint32
	err = windows.LookupAccountSid(nil, tokenUser.User.Sid, nil, &nameLen, nil, &domainLen, &use)
	if err != nil && err != windows.ERROR_INSUFFICIENT_BUFFER {
		return "", "", err
	}
	if nameLen == 0 {
		return "", "", fmt.Errorf("empty account name for token")
	}

	nameBuf := make([]uint16, nameLen)
	domainBuf := make([]uint16, domainLen)

	var domainPtr *uint16
	if len(domainBuf) > 0 {
		domainPtr = &domainBuf[0]
	}

	if err := windows.LookupAccountSid(nil, tokenUser.User.Sid, &nameBuf[0], &nameLen, domainPtr, &domainLen, &use); err != nil {
		return "", "", err
	}

	return windows.UTF16ToString(domainBuf[:domainLen]), windows.UTF16ToString(nameBuf[:nameLen]), nil
}

func matchWindowsUser(requested string, domain string, name string) bool {
	requestedDomain, requestedName := splitWindowsAccount(requested)
	if requestedName == "" {
		return false
	}
	if !strings.EqualFold(requestedName, name) {
		return false
	}
	return requestedDomain == "" || requestedDomain == "." || strings.EqualFold(requestedDomain, domain)
}

func splitWindowsAccount(account string) (domain string, name string) {
	account = strings.TrimSpace(strings.ReplaceAll(account, "/", "\\"))
	if account == "" {
		return "", ""
	}
	if idx := strings.LastIndex(account, "\\"); idx >= 0 {
		return strings.TrimSpace(account[:idx]), strings.TrimSpace(account[idx+1:])
	}
	if idx := strings.Index(account, "@"); idx > 0 {
		return strings.TrimSpace(account[idx+1:]), strings.TrimSpace(account[:idx])
	}
	return "", account
}
