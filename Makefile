# Cross-compile from Linux/macOS: make
dist/ngw.exe: $(shell find . -name '*.go')
	GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w -H windowsgui" -o $@ ./cmd/ngw

test:
	go test ./...
	GOOS=windows go vet ./internal/gui ./internal/nvim ./cmd/ngw

.PHONY: test
