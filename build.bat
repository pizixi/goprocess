go mod tidy
SET GOARCH=amd64
go build -ldflags "-s -w"
SET GOOS=linux
go build -a -ldflags "-extldflags '-static' -s -w" .
pause