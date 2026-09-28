VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build kde check test demo release clean

build:            ## hansei (Go: core, TUI, daemon) into bin/
	go build -trimpath -ldflags "$(LDFLAGS)" -o bin/hansei ./cmd/hansei

kde:              ## the KDE app into kde/build/bin/
	cmake -S kde -B kde/build -DCMAKE_BUILD_TYPE=Release -G Ninja
	cmake --build kde/build

test:
	go test ./...

check:            ## gofmt, vet, tests
	test -z "$$(gofmt -l core tui cmd internal)"
	go vet ./... && go vet -tags demo ./...
	go test -race ./...

demo:             ## internal: demo with Studio Weber data (demo/start.sh de|en app|tui)
	demo/start.sh de

release:          ## builds what ships and fails on demo leftovers
	scripts/check-release.sh $(VERSION)

clean:
	rm -rf bin build-demo kde/build kde/build-demo
