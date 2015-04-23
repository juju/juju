#!/bin/bash

LOCAL_REPO=$(readlink -f $1)  # Path to the local repo: gogo/src/github.com/foo
REMOTE_REPO=$(echo "$LOCAL_REPO" | sed -e 's,.*/src/,,')

: ${CI_TOOLS=$(readlink -f $(dirname $0))}
CLOUD_CITY=$(readlink -f $CI_TOOLS/../cloud-city)

while [[ "${1-}" != "" ]]; do
    case $1 in
        --cloud-city)
            shift
            CLOUD_CITY=$(readlink -f $1)
            ;;
    esac
    shift
done

cd $LOCAL_REPO
git co master
OLD_HASH=$(git log --first-parent -1 --pretty=format:%h)
source $CLOUD_CITY/juju-bot.txt
git pull https://$github_user:$github_password@$REMOTE_REPO.git master
NEW_HASH=$(git log --first-parent -1 --pretty=format:%h)
if [[ $OLD_HASH == $NEW_HASH ]]; then
    echo "Nothing to test."
    exit 1
else
    echo "A new revision can be tested."
    exit 0
fi
