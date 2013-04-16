#!/bin/bash

# basic stress test

set -e

while true; do
	go get -u -v launchpad.net/juju-core/utils
	export GOMAXPROCS=$[ 1 + $[ RANDOM % 128 ]]
        go test launchpad.net/juju-core/... 2>&1
done
