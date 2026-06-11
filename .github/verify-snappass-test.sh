#!/usr/bin/env bash

set -eux

retries=$1

attempts=0
while true; do
  if [ "$attempts" -eq "$retries" ]; then
    echo "Snappass health check timed out"
    exit 1
  fi
  attempts=$((attempts+1))

  snappass_status=$(sg snap_microk8s "microk8s kubectl -n m get -o json pod/snappass-test-0 | jq '.status | .containerStatuses[2] | .ready'" || echo "")
  [ "$snappass_status" == "true" ] && break

  sleep 10
done
