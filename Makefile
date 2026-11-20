BIN := spareglass

.PHONY: run build test vet fmt doctor snapshot clean

run:            ## start the host
	go run ./cmd/$(BIN) serve -v

build:
	go build -o bin/$(BIN) ./cmd/$(BIN)

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

doctor:
	go run ./cmd/$(BIN) doctor

snapshot: third_party/macos/ffmpeg  ## build release archives locally into dist/ (needs goreleaser)
	goreleaser release --snapshot --clean

third_party/macos/ffmpeg: packaging/macos/build-ffmpeg.sh
	packaging/macos/build-ffmpeg.sh third_party/macos

clean:
	rm -rf bin dist third_party
