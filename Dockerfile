FROM arm64v8/alpine as build
RUN apk update && apk add --no-cache make go
WORKDIR /opt
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make 
###
FROM arm64v8/alpine 
LABEL rpi_exporter.author="cavaliercoder@github" maintainer="Mr.Philipp <d3vilh@github.com>"
EXPOSE 9110
WORKDIR /opt
COPY --from=build /opt/rpi_exporter /opt
CMD ["./rpi_exporter", "-addr=:9110"]
