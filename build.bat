echo off
chcp 65001
echo 正在编译...

echo 正在编译Windows版本...
echo 生成ico资源文件...
windres -o rsrc.syso rsrc.rc
go mod tidy
SET GOOS=windows
SET GOARCH=amd64
SET CGO_ENABLED=1
go build -ldflags "-H windowsgui -s -w" -o goprocess.exe

echo 正在编译Linux版本...
SET GOOS=linux
SET GOARCH=amd64
SET CGO_ENABLED=0
go build -a -ldflags "-extldflags '-static' -s -w" -o goprocess-linux

echo 编译完成！
pause