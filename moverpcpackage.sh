#!/bin/bash

JUJU_PATH=github.com/juju/juju

function movepackage() {
    >&2 echo "  moving $JUJU_PATH/$1 to $JUJU_PATH/$2..."
    mv $1 $2
    for f in $(grep -lrF "$JUJU_PATH/$1" .); do
        sed -i "s~\"$JUJU_PATH/$1\"~\"$JUJU_PATH/$2\"~" "$f" # package
        sed -i "s~\"$JUJU_PATH/$1/~\"$JUJU_PATH/$2/~" "$f"   # subpackages
    done
}


movepackage "apiserver/params" "rpc/params"

gofmt -s -w .
