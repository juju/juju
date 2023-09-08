#!/usr/bin/bash

set -eux

retries=$1

attempts=0
while true; do
  if [ "$attempts" -eq "$retries" ]; then
    echo "Prometheus health check timed out"
    exit 1
  fi
  attempts=$((attempts+1))

  ip=$(juju status --format json | jq -r '.applications."prometheus-k8s".address' || echo "")
  port="9090"
  app_status=$(curl --silent --max-time 3 "${ip}:${port}/-/ready" || echo "")
  [ "$app_status" == "Prometheus Server is Ready." ] && break

  sleep 10
done
