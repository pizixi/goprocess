//go:build windows

package main

import (
	"fmt"
	"unsafe"

	// "WailsUIDemo/internal/logger"

	"golang.org/x/sys/windows"
)

var (
	modwtsapi32                      = windows.NewLazySystemDLL("wtsapi32.dll")
	modkernel32                      = windows.NewLazySystemDLL("kernel32.dll")
	modadvapi32                      = windows.NewLazySystemDLL("advapi32.dll")
	moduserenv                       = windows.NewLazySystemDLL("userenv.dll")
	procWTSEnumerateSessionsW        = modwtsapi32.NewProc("WTSEnumerateSessionsW")
	procWTSGetActiveConsoleSessionId = modkernel32.NewProc("WTSGetActiveConsoleSessionId")
	procWTSQueryUserToken            = modwtsapi32.NewProc("WTSQueryUserToken")
	procDuplicateTokenEx             = modadvapi32.NewProc("DuplicateTokenEx")
	procCreateEnvironmentBlock       = moduserenv.NewProc("CreateEnvironmentBlock")
	procCreateProcessAsUser          = modadvapi32.NewProc("CreateProcessAsUserW")
	procGetTokenInformation          = modadvapi32.NewProc("GetTokenInformation")
	procWTSFreeMemory                = modwtsapi32.NewProc("WTSFreeMemory")
)

type WTS_CONNECTSTATE_CLASS int
type SECURITY_IMPERSONATION_LEVEL int
type TOKEN_TYPE int
type SW int

type WTS_SESSION_INFO struct {
	SessionID      windows.Handle
	WinStationName *uint16
	State          WTS_CONNECTSTATE_CLASS
}

type TOKEN_LINKED_TOKEN struct {
	LinkedToken windows.Token
}

const (
	WTS_CURRENT_SERVER_HANDLE uintptr = 0
)

const (
	WTSActive WTS_CONNECTSTATE_CLASS = iota
	WTSConnected
	WTSConnectQuery
	WTSShadow
	WTSDisconnected
	WTSIdle
	WTSListen
	WTSReset
	WTSDown
	WTSInit
)

const (
	SecurityAnonymous SECURITY_IMPERSONATION_LEVEL = iota
	SecurityIdentification
	SecurityImpersonation
	SecurityDelegation
)

const (
	TokenPrimary TOKEN_TYPE = iota + 1
	TokenImpersonation
)

const (
	SW_HIDE            SW = 0
	SW_SHOWNORMAL      SW = 1
	SW_NORMAL          SW = 1
	SW_SHOWMINIMIZED   SW = 2
	SW_SHOWMAXIMIZED   SW = 3
	SW_MAXIMIZE        SW = 3
	SW_SHOWNOACTIVATE  SW = 4
	SW_SHOW            SW = 5
	SW_MINIMIZE        SW = 6
	SW_SHOWMINNOACTIVE SW = 7
	SW_SHOWNA          SW = 8
	SW_RESTORE         SW = 9
	SW_SHOWDEFAULT     SW = 10
	SW_MAX             SW = 11
)

const (
	CREATE_UNICODE_ENVIRONMENT uint32 = 0x00000400
	CREATE_NO_WINDOW           uint32 = 0x08000000
	CREATE_NEW_CONSOLE         uint32 = 0x00000010
	NORMAL_PRIORITY_CLASS      uint32 = 0x00000020
)

// GetCurrentUserSessionId 获取当前系统活动的 SessionID
func GetCurrentUserSessionId() (windows.Handle, error) {
	sessionList, err := WTSEnumerateSessions()
	if err == nil {
		for i := range sessionList {
			if sessionList[i].State == WTSActive {
				return sessionList[i].SessionID, nil
			}
		}
	}

	sessionId, _, err := procWTSGetActiveConsoleSessionId.Call()
	if sessionId == 0xFFFFFFFF {
		return 0xFFFFFFFF, fmt.Errorf("未找到活动的用户会话")
	}
	return windows.Handle(sessionId), nil
}

// WTSEnumerateSessions 枚举所有会话
func WTSEnumerateSessions() ([]*WTS_SESSION_INFO, error) {
	var (
		sessionInformation windows.Handle = 0
		sessionCount       int            = 0
		sessionList        []*WTS_SESSION_INFO
	)

	returnCode, _, err := procWTSEnumerateSessionsW.Call(
		WTS_CURRENT_SERVER_HANDLE,
		0,
		1,
		uintptr(unsafe.Pointer(&sessionInformation)),
		uintptr(unsafe.Pointer(&sessionCount)),
	)

	if returnCode == 0 {
		return nil, fmt.Errorf("call native WTSEnumerateSessionsW: %s", err)
	}

	defer procWTSFreeMemory.Call(uintptr(sessionInformation))

	structSize := unsafe.Sizeof(WTS_SESSION_INFO{})
	current := uintptr(sessionInformation)
	for i := 0; i < sessionCount; i++ {
		sessionList = append(sessionList, (*WTS_SESSION_INFO)(unsafe.Pointer(current)))
		current += structSize
	}

	return sessionList, nil
}

// DuplicateUserTokenFromSessionID 复制指定会话的用户令牌
func DuplicateUserTokenFromSessionID(sessionId windows.Handle, runas bool) (windows.Token, error) {
	var impersonationToken windows.Handle = 0
	var userToken windows.Token = 0

	returnCode, _, err := procWTSQueryUserToken.Call(
		uintptr(sessionId),
		uintptr(unsafe.Pointer(&impersonationToken)),
	)

	if returnCode == 0 {
		return 0xFFFFFFFF, fmt.Errorf("call native WTSQueryUserToken: %s", err)
	}
	defer windows.CloseHandle(impersonationToken)

	returnCode, _, err = procDuplicateTokenEx.Call(
		uintptr(impersonationToken),
		0,
		0,
		uintptr(SecurityImpersonation),
		uintptr(TokenPrimary),
		uintptr(unsafe.Pointer(&userToken)),
	)

	if returnCode == 0 {
		return 0xFFFFFFFF, fmt.Errorf("call native DuplicateTokenEx: %s", err)
	}

	if runas {
		var admin TOKEN_LINKED_TOKEN
		var dt uintptr = 0
		returnCode, _, _ := procGetTokenInformation.Call(
			uintptr(impersonationToken),
			19, // TokenLinkedToken
			uintptr(unsafe.Pointer(&admin)),
			uintptr(unsafe.Sizeof(admin)),
			uintptr(unsafe.Pointer(&dt)),
		)
		if returnCode != 0 && admin.LinkedToken != 0 {
			windows.Close(windows.Handle(userToken))
			userToken = admin.LinkedToken
		}
	}

	return userToken, nil
}

// StartProcessAsCurrentUser 以当前登录用户的身份启动进程
func StartProcessAsCurrentUser(appPath, cmdLine, workDir string, runas bool) error {
	var (
		sessionId windows.Handle
		userToken windows.Token
		envInfo   windows.Handle

		startupInfo windows.StartupInfo
		processInfo windows.ProcessInformation

		commandLine uintptr = 0
		workingDir  uintptr = 0

		err error
	)

	if sessionId, err = GetCurrentUserSessionId(); err != nil {
		return fmt.Errorf("get current user session id: %w", err)
	}

	if userToken, err = DuplicateUserTokenFromSessionID(sessionId, runas); err != nil {
		return fmt.Errorf("get duplicate user token for current user session: %w", err)
	}
	defer windows.Close(windows.Handle(userToken))

	returnCode, _, err := procCreateEnvironmentBlock.Call(
		uintptr(unsafe.Pointer(&envInfo)),
		uintptr(userToken),
		0,
	)

	if returnCode == 0 {
		return fmt.Errorf("create environment details for process: %s", err)
	}
	defer windows.DestroyEnvironmentBlock((*uint16)(unsafe.Pointer(envInfo)))

	creationFlags := CREATE_UNICODE_ENVIRONMENT | CREATE_NO_WINDOW | NORMAL_PRIORITY_CLASS
	startupInfo.Cb = uint32(unsafe.Sizeof(startupInfo))
	startupInfo.ShowWindow = uint16(SW_HIDE)
	startupInfo.Flags = windows.STARTF_USESHOWWINDOW
	startupInfo.Desktop = windows.StringToUTF16Ptr("winsta0\\default")

	if len(cmdLine) > 0 {
		commandLine = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(cmdLine)))
	}
	if len(workDir) > 0 {
		workingDir = uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(workDir)))
	}

	returnCode, _, err = procCreateProcessAsUser.Call(
		uintptr(userToken),
		uintptr(unsafe.Pointer(windows.StringToUTF16Ptr(appPath))),
		commandLine,
		0,
		0,
		0,
		uintptr(creationFlags),
		uintptr(envInfo),
		workingDir,
		uintptr(unsafe.Pointer(&startupInfo)),
		uintptr(unsafe.Pointer(&processInfo)),
	)

	if returnCode == 0 {
		return fmt.Errorf("create process as user: %s", err)
	}

	windows.CloseHandle(processInfo.Process)
	windows.CloseHandle(processInfo.Thread)

	fmt.Printf("Process started successfully: PID=%d", processInfo.ProcessId)
	return nil
}
