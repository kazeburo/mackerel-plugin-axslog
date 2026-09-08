VERSION=0.4.17
GITCOMMIT?=$(shell git describe --dirty --always)
LDFLAGS=-ldflags "-w -s -X main.version=${VERSION} -X main.commit=${GITCOMMIT}"

all: mackerel-plugin-axslog

.PHONY: mackerel-plugin-axslog

mackerel-plugin-axslog: main.go parser.go axslog/*.go jsonreader/*.go ltsvreader/*.go
	go build $(LDFLAGS) -o mackerel-plugin-axslog

linux: main.go parser.go axslog/*.go jsonreader/*.go ltsvreader/*.go
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o mackerel-plugin-axslog

check:
	go test -v ./...
	go test -race ./...

lint:
	golangci-lint run --timeout 5m ./...

bench:
	go test -bench BenchmarkParse -benchmem -run '^$$' .
	go test -bench BenchmarkMainParse -benchmem -run '^$$' .
