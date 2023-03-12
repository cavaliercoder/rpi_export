MAINTAINER d3vilh@github
LABEL rpi_exporter.author="cavaliercoder@github"
FROM arm64v8/debian
ARG OS=linux
ARG ARCH=arm64v8
ARG DEBIAN_FRONTEND=noninteractive
EXPOSE 9110
WORKDIR /opt
ADD rpi_exporter /opt/rpi_exporter
RUN chmod +x /opt/rpi_exporter
ENTRYPOINT /opt/rpi_exporter -addr=:9110
