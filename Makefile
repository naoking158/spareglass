BIN := spareglass

.PHONY: run build test vet fmt doctor snapshot clean

run:            ## start the host
	go run ./cmd/$(BIN) serve -v

build:
	go build -o bin/$(BIN) ./cmd/$(BIN)

test:
	go test -race ./...
	cd internal/virtualdisplay && go test -run TestCameUp .

vet:
	go vet ./...

fmt:
	gofmt -w .

doctor:
	go run ./cmd/$(BIN) doctor

snapshot: third_party/macos/ffmpeg third_party/windows/ffmpeg.exe third_party/windows/VirtualDisplayDriver-setup.exe  ## build release archives into dist/ (needs goreleaser, macOS, mingw-w64, gh)
	goreleaser release --snapshot --clean

third_party/macos/ffmpeg: packaging/macos/build-ffmpeg.sh
	packaging/macos/build-ffmpeg.sh third_party/macos

third_party/windows/ffmpeg.exe: packaging/windows/build-ffmpeg.sh packaging/windows/ffmpeg.manifest
	packaging/windows/build-ffmpeg.sh third_party/windows

third_party/windows/VirtualDisplayDriver-setup.exe: packaging/windows/fetch-vdd.sh
	packaging/windows/fetch-vdd.sh third_party/windows

clean:
	rm -rf bin dist third_party
