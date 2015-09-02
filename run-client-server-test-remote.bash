#!/bin/bash
set -eu

export JUJU_HOME=$HOME/cloud-city
export JUJU_REPOSITORY=$HOME/repository
export SCRIPT=$HOME/juju-ci-tools
export PATH=$HOME/juju-ci-tools:$PATH
set -x
usage() {
    echo "usage: $0 server client agent-arg agent-value"
    exit 1
}
test $# -eq 4 || usage
server="$1"
client="$2"
agent_arg="$3"
agent_arg_value="$4"

log_dir=$HOME/log
ssh_home="/tmp/sshhome"
mkdir -p $ssh_home/.ssh
cp $JUJU_HOME/staging-juju-rsa $ssh_home/.ssh/id_rsa
cp $JUJU_HOME/staging-juju-rsa.pub $ssh_home/.ssh/id_rsa.pub
export HOME=$ssh_home

mkdir server
mkdir client
tar zxf $server -C server
tar zxf $client -C client
server_juju=$(find server -name juju)
client_juju=$(find client -name juju)

env=compatibility-control
~/Bin/juju destroy-environment --force -y $env || true
$SCRIPT/assess_heterogeneous_control.py $server_juju $client_juju \
    test-osx-client-server $env $log_dir $agent_arg $agent_arg_value
