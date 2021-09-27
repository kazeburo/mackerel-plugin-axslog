VERSION=0.2.11
LDFLAGS=-ldflags "-w -s -X main.version=${VERSION}"

all: mackerel-plugin-axslog

.PHONY: mackerel-plugin-axslog

mackerel-plugin-axslog: main.go
	go build $(LDFLAGS) -o mackerel-plugin-axslog

linux: main.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o mackerel-plugin-axslog

check:
	go test ./...

fmt:
	go fmt ./...

tag:
	git tag v${VERSION}
	git push origin v${VERSION}
	git push origin master
