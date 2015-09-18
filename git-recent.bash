#!/bin/bash

REPO=$1
shift
PRIORITIES=$@

cd $REPO
git fetch -q 2>&1>/dev/null
RECENT=$(git for-each-ref \
    --format='gitbranch:%(refname:short):github.com/juju/juju' \
    --sort -committerdate --count 11 refs/remotes/origin/ | \
    sed -e '/HEAD/d; s,origin/,,' | sort --reverse | tr -s '\n' ' ')
echo "$PRIORITIES $RECENT"
