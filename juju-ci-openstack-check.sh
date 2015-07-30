#!/bin/bash
set -eu
usage() {
    echo "usage: $0 JUJU_ENVIRONMENT"
    echo "  JUJU_ENVIRONMENT: The juju environment to check."
    exit 1
}

test $# -eq 1 ||  usage
export OS_USERNAME=admin
export OS_PASSWORD=openstack
export OS_TENANT_NAME=admin
export OS_REGION_NAME=RegionOne
export OS_AUTH_URL=${OS_AUTH_PROTOCOL:-http}://`juju-deployer -e $1 -f keystone`:5000/v2.0

`dirname $0`/openstack_basic_check.py
