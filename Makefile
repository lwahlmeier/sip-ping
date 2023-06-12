VERSION ?= v0.0.0-local
GO_VERSION ?= 1.19
GO_FILES := $(shell find ./* -iname '*.go')
GO_RUN := docker run --rm -e GO111MODULE=on -e CGO_ENABLED=0 -e VERSION=$(VERSION) -e HOME=/build/.cache -u $$(id -u $${USER}):$$(id -g $${USER}) -v $$PWD:/build -w /build golang:$(GO_VERSION)
.PHONY: build clean

build: bin/linux-amd64/sip-ping bin/windows-amd64/sip-ping.exe bin/darwin-amd64/sip-ping bin/darwin-arm64/sip-ping

bin/linux-amd64/sip-ping: $(GO_FILES)
	$(GO_RUN) bash -c 'CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -mod=vendor -v -ldflags="-s -w" -o ./bin/linux-amd64/sip-ping'
bin/windows-amd64/sip-ping.exe: $(GO_FILES)
	$(GO_RUN) bash -c 'CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -mod=vendor -v -ldflags="-s -w" -o ./bin/windows-amd64/sip-ping.exe'
bin/darwin-arm64/sip-ping: $(GO_FILES)
	$(GO_RUN) bash -c 'CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -mod=vendor -v -ldflags="-s -w" -o ./bin/darwin-arm64/sip-ping'
bin/darwin-amd64/sip-ping: $(GO_FILES)
	$(GO_RUN) bash -c 'CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -mod=vendor -v -ldflags="-s -w" -o ./bin/darwin-amd64/sip-ping'

clean:
	rm -rf ./bin
