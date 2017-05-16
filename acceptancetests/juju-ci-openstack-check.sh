#!/bin/bash
set -eu
usage() {
    echo "usage: $0 JUJU_ENVIRONMENT"
    echo "  JUJU_ENVIRONMENT: The juju environment to check."
    exit 1
}

SCRIPTS=$(dirname $0)

test $# -eq 1 ||  usage
export OS_USERNAME=admin
export OS_PASSWORD=openstack
export OS_TENANT_NAME=admin
export OS_REGION_NAME=RegionOne
echo "PATH: $PATH"
echo "JUJU_HOME: $JUJU_HOME"
echo "Juju is $(which juju)"
echo "juju version is $(juju version)"
KEYSTONE_URL=$(juju deployer -e $1 -f keystone)
if [[ -z $KEYSTONE_URL ]]; then
    echo "Keystone URL could not be retrieved."
    echo "Openstack might be fine, but the call to deployer or juju failed."
    exit 1
fi
export OS_AUTH_URL=${OS_AUTH_PROTOCOL:-http}://$KEYSTONE_URL:5000/v2.0


$SCRIPTS/openstack_basic_check.py
