#!/bin/bash

ECO_BRANCH=$(readlink -f $1)  # Path to the repo: ~/Work/foo
ECO_PROJECT=$2  # Go Package name: github.com/juju/foo

: ${CI_TOOLS=$(readlink -f $(dirname $0))}
CLOUD_CITY=$(readlink -f $CI_TOOLS/../cloud-city)

while [[ "${1-}" != "" ]]; do
    case $1 in
        --cloud-city)
            shift
            CLOUD_CITY=$1
            ;;
    esac
    shift
done

cd $ECO_BRANCH
OLD_HASH=$(git log --first-parent -1 --pretty=format:%h)
source $CLOUD_CITY/juju-bot.txt
git pull https://$github_user:$github_password@$ECO_PROJECT.git
NEW_HASH=$(git log --first-parent -1 --pretty=format:%h)
if [[ $OLD_HASH == $NEW_HASH ]]; then
    echo "Nothing to test."
    exit 1
else
    echo "A new revision can be tested."
    exit 0
fi
