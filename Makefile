VERSION=0.4.5
LDFLAGS=-ldflags "-w -s -X main.version=${VERSION}"

all: mackerel-plugin-axslog

.PHONY: mackerel-plugin-axslog

mackerel-plugin-axslog: main.go parser.go axslog/*.go jsonreader/*.go ltsvreader/*.go
	go build $(LDFLAGS) -o mackerel-plugin-axslog

linux: main.go parser.go axslog/*.go jsonreader/*.go ltsvreader/*.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o mackerel-plugin-axslog

check:
	go test ./...
	go test -race ./...

fmt:
	go fmt ./...

tag:
	git tag v${VERSION}
	git push origin v${VERSION}
	git push origin master
