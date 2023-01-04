all: rpi_exporter

# Default: Linux on Raspberry Pi OS
rpi_exporter:
	GOOS=linux \
	GOARCH=arm \
	GOARM=7 \
	go build -o . ./...
