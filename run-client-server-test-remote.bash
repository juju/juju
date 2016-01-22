#!/bin/bash
set -eu

export JUJU_HOME=$HOME/cloud-city
export JUJU_REPOSITORY=$HOME/repository
export SCRIPT=$HOME/juju-ci-tools
export PATH=$HOME/juju-ci-tools:$PATH
usage() {
    echo "usage: $0 candidate-version old-juju-version new-to-old log-dir [--agent-stream stream | --agent-url url]"
    exit 1
}
test $# -ge 4 || usage
set -x
candidate_version="$1"
old_juju_version="$2"
new_to_old="$3"
log_dir="$4"
shift 4

# Extract the client and the server.
mkdir candidate
mkdir old-juju
if [[ -f $HOME/old-juju/osx/juju-$candidate_version-osx.tar.gz ]]; then
    candidate_juju=$HOME/old-juju/osx/juju-$candidate_version-osx.tar.gz
else
    candidate_juju=$HOME/candidate/osx/juju-$candidate_version-osx.tar.gz
fi
tar zxf $candidate_juju -C candidate
tar zxf $HOME/old-juju/osx/juju-$old_juju_version-osx.tar.gz -C old-juju

# Create ssh home.
ssh_home="/tmp/sshhome"
mkdir -p $ssh_home/.ssh
cp $JUJU_HOME/staging-juju-rsa $ssh_home/.ssh/id_rsa
cp $JUJU_HOME/staging-juju-rsa.pub $ssh_home/.ssh/id_rsa.pub
export HOME=$ssh_home


if [[ "$new_to_old" == "true" ]]; then
  server=$(find candidate -name juju)
  client=$(find old-juju -name juju)
else
  server=$(find old-juju -name juju)
  client=$(find candidate -name juju)
fi
echo "Server: " `$server --version`
echo "Client: " `$client --version`

mkdir $log_dir
env=compatibility-control-osx
$SCRIPT/assess_heterogeneous_control.py $server $client \
  parallel-reliability-aws $env $log_dir "$@"
