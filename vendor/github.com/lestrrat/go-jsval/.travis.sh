#!/bin/bash

set -e

DIFF=$(git diff)
if [[ ! -z "$DIFF" ]]; then
    echo "git diff found modified source after code generation"
    echo "$DIFF"
    exit 1
fi

go test -v ./...
