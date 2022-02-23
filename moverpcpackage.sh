#!/bin/bash

JUJU_PATH=github.com/juju/juju

function movepackage() {
    >&2 echo "  moving $JUJU_PATH/$1 to $JUJU_PATH/$2..."
    mv $1 $2
    for f in $(grep -lrF "$JUJU_PATH/$1" .); do
        sed -i "s~\"$JUJU_PATH/$1\"~\"$JUJU_PATH/$2\"~" "$f" # package
        sed -i "s~\"$JUJU_PATH/$1/~\"$JUJU_PATH/$2/~" "$f"   # subpackages
    done
}


# There are some tests in package_test.go files that check the package
# dependencies against a hard-coded list of expected values.  This function
# adjusts the hard-coded values if necessary.
#
# This function is idempotent
function adjusttests() {
    path='"apiserver/params"'
    repl='"rpc/params"'
    for file in $(grep -lrF --include package_test.go "$path"); do
        echo "$file: $path => $repl"
        sed -i "s~$path~$repl~" "$file"
    done
}

movepackage "apiserver/params" "rpc/params"
adjusttests
gofmt -s -w .


