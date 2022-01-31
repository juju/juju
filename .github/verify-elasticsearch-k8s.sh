#!/usr/bin/bash

set -euxo pipefail

retries=$1

attempts=0
while true; do
  if [ "$attempts" -eq "$retries" ]; then
    echo "Elasticsearch health check timed out"
    exit 1
  fi
  attempts=$((attempts+1))

  ip=$(juju status --format json | jq -r '.applications."elasticsearch-k8s".units[].address' || echo "")
  app_status=$(curl --silent --max-time 3 "${ip}:9200/_cluster/health" | jq -r ".status" || echo "")
  [ "$app_status" == "green" ] && break

  sleep 10
done
