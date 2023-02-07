all: rpi_exporter

# Default: Linux on Raspberry Pi OS
rpi_exporter:
	GOOS=linux \
	GOARCH=arm \
	GOARM=7 \
	go build -o . ./...

install: rpi_exporter
	install \
		-m 755 \
		-o node_exporter \
		-g node_exporter \
		rpi_exporter \
		/opt/node_exporter/rpi_exporter

clean:
	rm -f rpi_exporter

.PHONY: all clean install

