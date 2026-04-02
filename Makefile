APP_NAME=go-tracto.exe
export CGO_ENABLED=1

build: check
	go build -o $(APP_NAME) .\main.go

run: check
	go run .\main.go

check:
	@where gcc >nul 2>&1 || (echo "Error: GCC not found in PATH" && exit 1)

clean:
	@if exist $(APP_NAME) del $(APP_NAME)