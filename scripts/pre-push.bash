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

        ./scripts/verify.bash

    fi
done

