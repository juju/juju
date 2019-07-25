#!/bin/bash

prepare_project() {
    apt-get update
    apt-get install -y build-essential

    cd $PROJECT_PATH

    make install-dependencies setup-lxd
    make release-install
}

restore_project() {
    # Remove all of the code we pushed and any build results. This removes
    # stale files and we cannot do incremental builds anyway so there's little
    # point in keeping them.
    if [ -n "$GOPATH" ]; then
        rm -rf "${GOPATH%%:*}"
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
