#!/bin/bash
# Copyright 2014 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

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

# Define additional Printf style functions to check. These add to the
# default list of standard library functions that go vet already has.
logging_prints="\
Tracef
Debugf
Infof
Warningf
Errorf
Criticalf
"

error_prints="\
AlreadyExistsf
BadRequestf
MethodNotAllowedf
NotAssignedf
NotFoundf
NotImplementedf
NotProvisionedf
NotSupportedf
NotValidf
Unauthorizedf
UserNotFoundf
"

# Under Go 1.6, the vet docs say that -printfuncs takes each print
# function in "name:N" format. This has changed in Go 1.7 and doesn't
# actually seem to make a difference under 1.6 either don't bother.
all_prints=`echo $logging_prints $error_prints | tr " " ,`

go tool vet \
   -all \
   -composites=false \
   -copylocks=false \
   -printfuncs=$all_prints \
    . || [ -n "$IGNORE_VET_WARNINGS" ]


echo "checking: go build ..."
go build github.com/juju/juju/...

echo "checking: tests are wired up ..."
./scripts/checktesting.bash
