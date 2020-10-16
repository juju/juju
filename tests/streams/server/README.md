# Simplestream Server

The Simplestream server creates a stub Simplestream server for handling the
serving of agent binaries a long with outputting the correct Simplestream json
files.

### Building

Building the server is rather easy and is preferred over `go run` if you require
a PID of the server. 

```sh
go build -o server ./tests/streams/server/main.go
```

### Running

Generating a stream for the simplestream requires a release configuration line:

 - `<stream>,<series>-<version>-<arch>,<juju version>,<path to jujud>`

#### Example

Create a 2.8.6 "released" stream targeting focal amd64. 

```sh
server \
    --release="released,focal-20.04-amd64,2.8.6,agent/2.8.6-focal-amd64.tar.gz" \
    ./tests/suites/bootstrap/streams/
```

The following targets both bionic and focal on "released" and "devel" with the
same binary.

```sh
server \
    --release="released,focal-20.04-amd64,2.8.6,agent/2.8.6-focal-amd64.tar.gz" \
    --release="released,bionic-18.04-amd64,2.8.6,agent/2.8.6-focal-amd64.tar.gz" \
    --release="devel,focal-20.04-amd64,2.8.6,agent/2.8.6-focal-amd64.tar.gz" \
    --release="devel,bionic-18.04-amd64,2.8.6,agent/2.8.6-focal-amd64.tar.gz" \
    ./tests/suites/bootstrap/streams/
```
