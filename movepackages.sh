#!/bin/bash

JUJU_PATH=github.com/juju/juju
API_GROUPS="agent client controller"

function findpackages() {
    find apiserver/facades/$1 -type d -maxdepth 1 -exec basename {} \;
}

function movepackage() {
    >&2 echo "  moving $JUJU_PATH/$1 to $JUJU_PATH/$2..."
    mv $1 $2
    for f in $(grep -lrF "$JUJU_PATH/$1" .); do
        sed -i "s~\"$JUJU_PATH/$1\"~\"$JUJU_PATH/$2\"~" "$f" # package
        sed -i "s~\"$JUJU_PATH/$1/~\"$JUJU_PATH/$2/~" "$f"   # subpackages
    done
}

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
        if [ -d "api/$p" ]; then
            echo "found"
            movepackage "api/$oldp" "api/$g/$p"
        else
            echo "not found"
        fi
    done
    >&2 echo "done moving packages to $g"
done

gofmt -s -w
