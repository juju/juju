#!/bin/bash
# This script reproduces bug #1680936.  It assumes a blank model.  It:
# - deploys ubuntu (to machine 0)
# - adds a container to machine 0
# - removes the container from machine 0
# - optionally waits for the container to be removed from status
# - removes ubuntu/0
# If machine 0 is removed, it succeeds.  If machine 0 is still there after 5
# minutes, it fails.
set -eux

function wait_for_null(){
  deadline="$(($(date +"%s") + 300))"
  set +x
  while [ $(date +"%s") -lt $deadline ]; do
    if [ "$(juju status --format json|jq $1)" == "null" ]; then
      echo
      echo "Query $1 went null."
      return 0
    fi
    echo -n '.'
    sleep 1
  done
  echo
  set +x
  return 1
}

juju deploy ubuntu
# Machine doesn't appear instantly
sleep 1
juju add-machine lxd:0
juju remove-machine 0/lxd/0
echo "Waiting for removal of machine 0/lxd/0"
if [ ${WAIT_LXD-false} == "true" ]; then
  wait_for_null '.machines."0".containers."0/lxd/0"'
fi
juju remove-unit ubuntu/0
echo "Waiting for removal of machine 0."
if wait_for_null '.machines."0"'; then
  echo "SUCCESS: machine 0 was removed."
  exit 0
else
  echo "FAILURE: machine 0 was not removed."
  juju status
  exit 1
fi
