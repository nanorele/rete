# Project Rete

### Lauch:

```
go run ./...
```

### Build

Windows

```
set GOAMD64=v3 && go build -gcflags="-B" -trimpath -ldflags="-s -w -H=windowsgui" -o bin\rete.exe cmd\main.go
```