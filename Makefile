APP_NAME=rete.exe
SYSO=cmd\rsrc_windows_amd64.syso

icon:
	cd assets\icon && windres -F pe-x86-64 -O coff rete.rc ..\..\$(SYSO)

build: icon
	go build -o $(APP_NAME) .\cmd

run:
	go run .\cmd

clean:
	@if exist $(APP_NAME) del $(APP_NAME)
	@if exist $(SYSO) del $(SYSO)

lint:
	golangci-lint run ./...