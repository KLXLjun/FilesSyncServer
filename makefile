APP=FilesSyncServer

BUILD_TIME=`date +%FT%T%z`
BUILD_TIME_WIN=`powershell -Command "Get-Date -Format yyyy-MM-dd_HH-mm-ss"`

LDFLAGS=-ldflags "-X main.BuildTime=${BUILD_TIME}"
LDFLAGS_WIN=-ldflags "-X main.BuildTime=${BUILD_TIME_WIN}"
ReleaseLDFLAGS=-ldflags "-s -w -X main.BuildTime=${BUILD_TIME}" -buildvcs=false
ReleaseLDFLAGS_WIN=-ldflags "-s -w -X main.BuildTime=${BUILD_TIME_WIN}" -buildvcs=false

all:
	make win
	make linux
win:
	CGO_ENABLED=0 GOOS=windows go build -o ./${APP}-windows.exe
linux:
	CGO_ENABLED=0 GOOS=linux go build -o ./${APP}-linux