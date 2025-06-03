#!/usr/bin/env bash

set -euxo pipefail

retries=$1

attempts=0
while true; do
  if [ "$attempts" -eq "$retries" ]; then
    echo "Apache health check timed out"
    exit 1
  fi
  attempts=$((attempts+1))

  ip=$(juju status --format json | jq -r '.applications.apache2.units[]."public-address"' || echo "")
  curl --silent --output /dev/null --max-time 3 "$ip" && break

  sleep 10
done
