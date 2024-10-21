#!/bin/bash

set -eu

secrets=$(juju secrets --format=json | jq -r "to_entries | .[] | select( .value | .revision > 1) | .key" | sort -u)
for secret in $secrets; do
    owner=$(juju show-secret $secret --format=json | jq -r ".[] | .owner")
    revision=$(juju show-secret $secret --format=json | jq -r ".[] | .revision")
    new_revision=$((revision-1))
    echo "$owner Secret: $secret at $revision"

    remove=0
    read -p "Do you wish to remove revision from 1 to $new_revision? " yn
    case $yn in
        [Yy]* ) remove=1;;
        [Nn]* ) ;;
        * ) echo "Please answer yes or no.";;
    esac

    if [ $remove = 0 ]; then
        continue
    fi

    for i in $(seq 1 $new_revision); do
        echo " - Removing $secret at revision $i"
        juju exec -u $owner/leader -- secret-remove "secret:$secret" --revision $i 2>&1 | sed "s/^/   > /" || true
    done
done
