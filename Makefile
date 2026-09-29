# Cross-platform build helpers for Single-Cell Virtual Cytometer.
#
# These targets use fyne-cross (https://github.com/fyne-io/fyne-cross),
# the official Fyne cross-compilation tool. It runs the actual build
# inside Docker images that already contain the right C toolchain for
# each target OS (MinGW for Windows, etc.) — this matters because Fyne
# uses CGO for native graphics, which plain `GOOS=windows go build`
# cannot cross-compile on its own.
#
# Prerequisites (one-time):
#   - Go >= 1.19
#   - Docker, running and usable by your user
#   - fyne-cross itself:  go install github.com/fyne-io/fyne-cross@latest
#     (make sure $(go env GOPATH)/bin is on your PATH)
#
# Usage:
#   make build-linux      # -> dist/scvc/linux-amd64/scvc  (or scvc.tar.xz)
#   make build-windows    # -> dist/scvc/windows-amd64/scvc.exe
#   make build-all        # both of the above
#   make clean            # remove the dist/ output
#
# If you're not on Linux/macOS (no `make` / no Docker available, e.g. a
# locked-down Windows machine), skip this file entirely and just run
#   go build ./cmd/scvc
# natively on each target machine instead — see the README.

APP_ID   := io.github.scvirtualcytometer
APP_NAME := scvc
OUT      := dist

.PHONY: build-linux build-windows build-all clean

build-linux:
	fyne-cross linux -arch=amd64 -app-id=$(APP_ID) -name=$(APP_NAME) -output=$(APP_NAME) ./cmd/scvc

build-windows:
	fyne-cross windows -arch=amd64 -app-id=$(APP_ID) -name=$(APP_NAME) -output=$(APP_NAME) ./cmd/scvc

build-all: build-linux build-windows

clean:
	rm -rf $(OUT)
