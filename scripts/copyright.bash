#!/bin/zsh

# Copyright 2013 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
exitstatus=0
result=$(find -name '*.go' | xargs head -n 1 | grep -v -E "==> (./vendor|acceptancetests)" |  grep -v -E '// (Copyright|Code generated)' | grep -Pzo "==> (.*) <==\n.*\n\n" | grep -Pzo "==> (.*) <==\n" | sed 's/==> \(.*\) <==/\1/')
missing=$(echo "$result" | wc -l)
if [ missing != 0 ]; then
    echo "The following files are missing copyright headers"
    echo "$result"
    exitstatus=1
fi
exit $exitstatus
