#!/usr/bin/bash

set -euxo pipefail

target_version="$1"
attempt=0
while true; do
  UPDATED=$((juju show-controller --format=json || echo "") | jq -r '.c.details."agent-version"')
  if [[ $UPDATED == $target_version* ]]; then
      break
  fi
  sleep 10
  attempt=$((attempt+1))
  if [ "$attempt" -eq 48 ]; then
      echo "Upgrade version check timed out"
      exit 1
  fi
done
