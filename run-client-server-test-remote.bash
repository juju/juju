#!/bin/bash
set -eu

export JUJU_HOME=$HOME/cloud-city
export JUJU_REPOSITORY=$HOME/repository
export SCRIPT=$HOME/juju-ci-tools
export PATH=$HOME/juju-ci-tools:$PATH
set -x
usage() {
    echo "usage: $0 server client [--agent-stream stream | --agent-url url]"
    exit 1
}
test $# -ge 2 || usage
server="$1"
client="$2"
shift 2

# Create ssh home.
ssh_home="/tmp/sshhome"
mkdir -p $ssh_home/.ssh
cp $JUJU_HOME/staging-juju-rsa $ssh_home/.ssh/id_rsa
cp $JUJU_HOME/staging-juju-rsa.pub $ssh_home/.ssh/id_rsa.pub
export HOME=$ssh_home

# Extract the client and the server.
tar zxf $server -C server
tar zxf $client -C client
server_juju=$(find server -name juju)
client_juju=$(find client -name juju)

mkdir logs
env=compatibility-control
~/Bin/juju destroy-environment --force -y $env || true
$SCRIPT/assess_heterogeneous_control.py $server_juju $client_juju \
    test-reliability-aws $env logs "$@"
