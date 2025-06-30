@echo off
chcp 65001
setlocal EnableDelayedExpansion

:: 获取当前批处理文件所在目录
set "current_dir=%~dp0"

:: 去除路径末尾的反斜杠
if "%current_dir:~-1%"=="\" set "current_dir=%current_dir:~0,-1%"

:: 构建goprocess.exe的完整路径
set "exe_path=%current_dir%\goprocess.exe"

:: 检查文件是否存在
if not exist "%exe_path%" (
    echo 错误：找不到文件 %exe_path%
    echo 请确保goprocess.exe文件位于当前目录中。
    pause
    exit /b 1
)

echo 准备添加注册表项...
echo 文件路径：%exe_path%

:: 添加注册表项到启动项
reg add "HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run" /v "goprocess" /t REG_SZ /d "\"%exe_path%\"" /f

:: 检查注册表添加是否成功
if %errorlevel% equ 0 (
    echo.
    echo 成功！goprocess.exe 已添加到Windows启动项。
    echo 注册表位置：HKEY_LOCAL_MACHINE\SOFTWARE\Microsoft\Windows\CurrentVersion\Run
    echo 字段名：goprocess
    echo 值：%exe_path%
) else (
    echo.
    echo 失败！无法添加注册表项。
    echo 请确保以管理员权限运行此批处理文件。
)

echo.
goprocess.exe