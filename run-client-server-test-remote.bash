#!/bin/bash
set -eu

usage() {
    echo "usage: $0 old-version candidate-version new-to-old"
    echo "Example: $0 1.21.1 1.24.3 false"
    exit 1
}
test $# -eq 3 || usage
old_version="$1"
candidate_version="$2"
new_to_old="$3"

export JUJU_HOME=$HOME/cloud-city
export JUJU_REPOSITORY=$HOME/repository
export SCRIPT=$HOME/juju-ci-tools
export PATH=$HOME/juju-ci-tools:$PATH

set -x
old_juju=$(find $HOME/old-juju/$old_version -name juju)
new_juju=$(find $HOME/candidate/$candidate_version -name juju)
log_dir=$HOME/log

if [ "$new_to_old" == "true" ]; then
  server=$new_juju
  client=$old_juju
else
  server=$old_juju
  client=$new_juju
fi

env=test-release-aws
ssh_home="/tmp/sshhome"
mkdir -p $ssh_home/.ssh
cp $JUJU_HOME/staging-juju-rsa $ssh_home/.ssh/id_rsa
cp $JUJU_HOME/staging-juju-rsa.pub $ssh_home/.ssh/id_rsa.pub
export HOME=$ssh_home

~/Bin/juju destroy-environment --force -y $env || true
$SCRIPT/assess_heterogeneous_control.py $server $client test-release-aws compatibility-control $log_dir
