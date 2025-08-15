#!/bin/bash
# Copyright 2013 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.

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

        # We always want to check against new branches, so we're only here
        # wanting to check to see if there is any difference between an updated
        # branch and the remote sha of that branch. If no files have changed,
        # skip the verify phase.
        if ! [ "$remote_sha" = $z40 ]; then
            # Update to existing branch, examine new commits
            range="$remote_sha...$local_sha"

            FILECOUNT=`git log --name-only '--pretty=format:' $range | grep '.go$' | wc -l`
            if [ $FILECOUNT -eq 0 ]; then
                # no go files changed, skip go validation
                exit 0
            fi
        fi

        ./scripts/verify.bash
    fi
done
