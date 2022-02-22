#!/bin/bash

JUJU_PATH=github.com/juju/juju
API_GROUPS="agent client controller"

function findpackages() {
    find apiserver/facades/$1 -mindepth 1 -maxdepth 1 -type d -exec basename {} \;
}

function movepackage() {
    >&2 echo "  moving $JUJU_PATH/$1 to $JUJU_PATH/$2..."
    mv $1 $2
    for f in $(grep -lrF "$JUJU_PATH/$1" .); do
        sed -i "s~\"$JUJU_PATH/$1\"~\"$JUJU_PATH/$2\"~" "$f" # package
        sed -i "s~\"$JUJU_PATH/$1/~\"$JUJU_PATH/$2/~" "$f"   # subpackages
    done
}

# Group packages in api/ by api group.  Not safe to run twice :)
function movepackages() {
    for g in $API_GROUPS; do
        if [[ -d api/$g ]]; then
            >&2 echo "api/$g exists, moving it to api/renamed$g"
            movepackage "api/$g" "api/renamed$g"
        fi
    done

    for g in $API_GROUPS; do
        >&2 echo "moving packages to $g"
        mkdir -p "api/$g"
        for p in $(findpackages "$g"); do
            oldp="$p"
            if [[ $p == $g ]]; then
                oldp="renamed$p"
            fi
            >&2 echo -n "  looking for $oldp... "
            if [ -d "api/$oldp" ]; then
                echo "found"
                movepackage "api/$oldp" "api/$g/$p"
            else
                echo "not found"
            fi
        done
        >&2 echo "done moving packages to $g"
    done

    movepackage "api/machiner" "api/agent/machiner"
    movepackage "api/pubsub" "api/controller/pubsub"

    gofmt -s -w .
}

# There are some tests in package_test.go files that check the package
# dependencies against a hard-coded list of expected values.  This function
# adjusts the hard-coded values if necessary.
#
# This function is idempotent
function adjusttests() {
    for l in $(grep -roE --include package_test.go '"api/[a-z]+"'); do
        file=$(echo "$l" | cut -d: -f1)
        path=$(echo "$l" | cut -d: -f2)
        repl=$(find api -maxdepth 2 -mindepth 2 -type d -name ${path:5:-1})
        if [[ $repl != "" ]]; then
            repl="\"$repl\""
            echo "$file: $path => $repl"
            sed -i "s~$path~$repl~" "$file"
        fi
    done
}

movepackages
adjusttests