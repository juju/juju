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

FILES=`find * -name '*.go' -not -name '.#*' | grep -v vendor/ | grep -v acceptancetests/`

echo "checking: dependency files ..."
dep check

echo "checking: copyright notices are in place ..."
./scripts/copyright.bash

echo "checking: go fmt ..."
BADFMT=`echo "$FILES" | xargs gofmt -l -s`
if [ -n "$BADFMT" ]; then
    BADFMT=`echo "$BADFMT" | sed "s/^/  /"`
    echo -e "gofmt failed, run the following command(s) to fix:\n"
    for item in $BADFMT; do
        echo "gofmt -l -s -w $item"
    done
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
Annotatef
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
DIRNAMES=$(dirname $FILES | sort -u | xargs -I % echo "github.com/juju/juju/%")

go vet \
   -all \
   -composites=false \
   -printfuncs=$all_prints \
    $DIRNAMES || [ -n "$IGNORE_VET_WARNINGS" ]

# Allow the ignoring of the golinters
if [ -z "$IGNORE_GOLINTERS" ]; then
    echo "checking: golinters ..."
    ./scripts/golinters.bash
else
    echo "ignoring: golinters ..."
fi

echo "checking: go build ..."
go build $(go list github.com/juju/juju/... | grep -v /vendor/)

echo "checking: tests are wired up ..."
./scripts/checktesting.bash
