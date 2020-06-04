#!/bin/sh
set -euf

ensure_dep() {
    if [ ! -f Gopkg.lock ]; then
       echo "using Go modules instead of dep"
       return
    fi

    if [ ! -d vendor ]; then
        SHA=$(sha256sum Gopkg.lock | cut -d ' ' -f 1)
        wget -O vendor.tar.gz "https://juju-dep.s3-eu-west-1.amazonaws.com/${SHA}.tar.gz"
        tar -xf vendor.tar.gz
        rm -rf vendor.tar.gz
        # Make sure that the fetched snapshot does indeed match the lock file
        echo "verifying contents of vendor snapshot for lock file with SHA ${SHA}"
        dep check
        echo "using cached vendor folder for lock file with SHA ${SHA}; skipping 'dep ensure'"
        touch .cached_vendor_deps
    elif [ -f .cached_vendor_deps ]; then
        echo "vendor folder populated from snapshot; skipping 'dep ensure'"
    else
        echo "running 'dep ensure' to make sure all dependencies are up to date"
        ${GOPATH}/bin/dep ensure -vendor-only
    fi
}
