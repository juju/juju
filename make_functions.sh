#!/bin/sh
set -euf

ensure_dep() {
    if [ ! -d vendor ]; then
        SHA=$(sha256sum Gopkg.lock | cut -d ' ' -f 1)
        wget -O vendor.tar.gz "https://juju-dep.s3-eu-west-1.amazonaws.com/${SHA}.tar.gz"
        tar -xvzf vendor.tar.gz
        rm -rf vendor.tar.gz
    else
        echo "Skipping..."
    fi
}
