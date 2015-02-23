#!/bin/bash

# This is called from pre-push.bash to do some verification checks on 
# the Go code.  The script will exit non-zero if any of these tests
# fail. However if environment variable IGNORE_VET_WARNINGS is a non-zero
# length string, go vet warnings will not exit non-zero.

set -e 

VERSION=`go version | awk '{print $3}'`
echo "go version $VERSION"

echo "checking: go fmt ..."
BADFMT=`find * -name '*.go' -not -name '.#*' | xargs gofmt -l`
if [ -n "$BADFMT" ]; then
    BADFMT=`echo "$BADFMT" | sed "s/^/  /"`
    echo -e "gofmt is sad:\n\n$BADFMT"
    exit 1
fi


echo "checking: go vet ..."
go tool vet \
    -methods \
    -printf \
    -rangeloops \
    -printfuncs 'ErrorContextf:1,notFoundf:0,badReqErrorf:0,Commitf:0,Snapshotf:0,Debugf:0,Infof:0,Warningf:0,Errorf:0,Criticalf:0,Tracef:0' \
    . || [ -n "$IGNORE_VET_WARNINGS" ]


echo "checking: go build ..."
# check this branch builds cleanly
go build github.com/juju/juju/...

echo "checking: tests are wired up ..."
# check that all tests are wired up
./scripts/checktesting.bash

