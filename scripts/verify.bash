#!/bin/bash
# Copyright 2014 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

# This is called from pre-push.bash to do some verification checks on
# the Go code. The script will exit non-zero if any of these tests
# fail.

set -e

VERSION=`go version | awk '{print $3}'`
echo "go version $VERSION"

STATIC_ANALYSIS="${STATIC_ANALYSIS:-}"
if [ -n "$STATIC_ANALYSIS" ]; then
    make static-analysis
else
    echo "Ignoring static analysis, run again with STATIC_ANALYSIS=1 ..."
fi
