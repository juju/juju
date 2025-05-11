#!/usr/bin/env bash

set -e

if [ "$#" -ne 1 ]; then
    echo "need one package to refactor"
fi

set -e

SCRIPT_DIR=$(dirname "$(realpath "$0")")

# Step 1: Run main.go over the package to refactor and make sure all public
# method start with a trace.
go run scripts/trace-patch/main.go "${1}"

# Step 4 fix up imports that have been modified by go patch.
gci  write --skip-generated -s standard -s default -s 'Prefix(github.com/juju/juju)' "${1}/."

# Step 5 remove unused imports that are going to blow up the compiler.
goimports -w "${1}/."
