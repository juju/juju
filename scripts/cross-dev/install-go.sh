#!/bin/bash
set -eux

case $(uname -m) in

  x86_64)
    GOARCH=amd64
    ;;

  amd64)
    GOARCH=amd64
    ;;

  arm64)
    GOARCH=arm64
    ;;

  aarch64)
    GOARCH=arm64
    ;;

  *)
    echo "Bad arch $(uname -m)"
    ;;
esac

GOTAR=$(curl -s https://go.dev/dl/ | grep -oE "go${GOVERSION}(\.[0-9]+)?\.linux-${GOARCH}.tar.gz" | head -n1)

wget -q "https://golang.org/dl/${GOTAR}"
rm /usr/local/bin/go /usr/local/bin/gofmt || true
rm -rf /usr/local/go || true
tar -C /usr/local -xzf "${GOTAR}"
rm "${GOTAR}"
ln -s /usr/local/go/bin/go /usr/local/bin/go
ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt

exit 0