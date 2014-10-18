#!/bin/bash

set -e

z40=0000000000000000000000000000000000000000

IFS=' '
while read local_ref local_sha remote_ref remote_sha
do
    if [ "$local_sha" = $z40 ]; then
        # delete remote branch, no check
        exit 0
    else
        git diff --quiet || (echo "unstaged changes"; exit 1)
        git diff --cached --quiet || (echo "uncommitted changes"; exit 1)

        if [ "$remote_sha" = $z40 ]
        then
            # New branch, examine all commits not on master
            range="$local_sha...master"
        else
            # Update to existing branch, examine new commits
            range="$remote_sha...$local_sha"
        fi

        FILECOUNT=`git log --name-only '--pretty=format:' $range | grep '.go$' | wc -l`
        if [ $FILECOUNT -eq 0 ]; then
            # no go files changed, skip go validation
            exit 0
        fi

        VERSION=`go version | awk '{print $3}'`
        echo "go version $VERSION"

        echo "checking: go fmt ..."
        BADFMT=`find * -name '*.go' -not -name '.#*' | xargs gofmt -l`
        if [ -n "$BADFMT" ]; then
            BADFMT=`echo "$BADFMT" | sed "s/^/  /"`
            echo -e "gofmt is sad:\n\n$BADFMT"
            exit 1
        fi


        echo "checking: go vet ..."
        go tool vet \
            -methods \
            -printf \
            -rangeloops \
            -printfuncs 'ErrorContextf:1,notFoundf:0,badReqErrorf:0,Commitf:0,Snapshotf:0,Debugf:0,Infof:0,Warningf:0,Errorf:0,Criticalf:0,Tracef:0' \
            .


        echo "checking: go build ..."
        # check this branch builds cleanly
        go build github.com/juju/juju/...

        echo "checking: tests are wired up ..."
        # check that all tests are wired up
        ./scripts/checktesting.bash

    fi
done

