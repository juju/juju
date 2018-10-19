#!/bin/bash

# Copyright 2013 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
exitstatus=0
result=$(find -name '*.go' | grep -v -E "(./vendor|./acceptancetests|./provider/azure/internal|./cloudconfig)" | sort | xargs head -n 1 | grep -v -E '// (Copyright|Code generated)' | xargs echo | sed 's/==>/\n==>/g' | sed 's/<==/<==\n/g' | grep -Pzo "==> (.*) <==\n.*\S+.*\n" | grep -Pzo "==> (.*) <==\n" | sed 's/==> \(.*\) <==/\1/')
missing=$(echo "$result" | wc -w)
if [ $missing != 0 ]; then
    echo "The following files are missing copyright headers"
    echo "$result"
    exitstatus=1
fi
exit $exitstatus
