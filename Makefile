export GOPROXY := https://goproxy.cn,direct
export GOSUMDB := sum.golang.google.cn

.PHONY: test run tidy

test:
	go test ./...

run:
	go run ./cmd/server

tidy:
	go mod tidy
