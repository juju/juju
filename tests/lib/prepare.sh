#!/bin/bash

set -eux

prepare_project() {
    cd $PROJECT_PATH

    make install-dependencies setup-lxd
    if [ -d "vendor" ]; then
        make go-install
    else
        make install
    fi
    if [ "$SPREAD_BACKEND" = qemu ]; then
        juju bootstrap lxd test --no-gui
    fi
}

restore_project() {
    # Remove all of the code we pushed and any build results. This removes
    # stale files and we cannot do incremental builds anyway so there's little
    # point in keeping them.
    if [ -n "$GOPATH" ]; then
        rm -rf "${GOPATH%%:*}"
    fi
    if [ command -v juju ]; then
        juju destroy-controller test --destroy-all-models -y
    fi
}

case "$1" in
    --prepare-project)
        prepare_project
        ;;
    --restore-project)
        restore_project
        ;;
    *)
        echo "unsupported argument: $1"
        echo "try one of --{prepare,restore}-{project}"
        exit 1
        ;;
esac
