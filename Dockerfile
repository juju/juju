# STEP 1 build executable binary
FROM golang:1.10 as builder

ARG PROJECT=github.com/juju/juju
WORKDIR $GOPATH/src/$PROJECT
COPY . .

RUN make dep verbose=-v

# CGO_ENABLED=0 to use scratch image but `github.com/lxc/lxd` does not work
RUN CGO_ENABLED=1 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -a -ldflags="-w -s" -o /go/bin/jujud -v github.com/juju/juju/cmd/jujud

# STEP 2 build image
# start from scratch
# FROM scratch
FROM ubuntu:bionic

# remove this later!!!
RUN apt update && apt install -y \
    iproute2 \
    ca-certificates

ARG JUJUD_DIR=/opt/juju/tools/machine-0
WORKDIR /var/lib/juju

COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
RUN update-ca-certificates

# Copy our static executable
COPY --from=builder /go/bin/jujud $JUJUD_DIR/

EXPOSE 17070

ENTRYPOINT ["sh", "-c"]
# TODO: HA mode - parse in machine name.
CMD ["/opt/juju/tools/machine-0/jujud machine --data-dir /var/lib/juju --machine-id 0 --debug"]
