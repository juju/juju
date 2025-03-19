FROM docker.io/golang:1.24 AS build

# Set the working directory.
WORKDIR /resource

# Copy the Go source code into the container.
COPY go.mod .
COPY main.go .

# Build the Go application.
RUN go mod tidy && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ./whoami-server

# This is a multistage build, only include whats needed in the final image.
FROM scratch

# Copy the binary over from the build stage.
COPY --from=build /resource/whoami-server /resource/whoami-server

# Copy pebble service definition to run whoami server.
COPY 001-layer.yaml /var/lib/pebble/default/layers/

# Expose the whoami server port.
EXPOSE 8080

# Run the server.
CMD ["/resource/whoami-server"]
