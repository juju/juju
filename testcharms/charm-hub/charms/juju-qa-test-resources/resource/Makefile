default:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../runnable_amd64.out main.go
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -o ../runnable_arm64.out main.go

.PHONY: default
