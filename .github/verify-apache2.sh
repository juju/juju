#!/usr/bin/bash

set -euxo pipefail

ip=$(juju status --format json | jq -r '.applications.apache2.units[]."public-address"')

curl --silent --output /dev/null --max-time 3 "$ip"
