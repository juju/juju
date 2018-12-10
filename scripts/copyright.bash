#!/bin/bash

# Copyright 2013 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
exitstatus=0
result=$(find . -name '*.go' | grep -v -E "(./vendor|./acceptancetests|./provider/azure/internal|./cloudconfig)" | sort | xargs grep -L -E '// (Copyright|Code generated)')
missing=$(echo "$result" | wc -w)
if [ $missing != 0 ]; then
    echo "The following files are missing copyright headers"
    echo "$result"
    exitstatus=1
fi
exit $exitstatus
