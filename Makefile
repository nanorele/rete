APP_NAME=go-tracto.exe

build:
	go build -o $(APP_NAME) .\cmd

run:
	go run .\cmd

clean:
	@if exist $(APP_NAME) del $(APP_NAME)