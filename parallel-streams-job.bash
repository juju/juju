#!/bin/bash
set -eu
source $HOME/cloud-city/juju-qa.jujuci
PATH="$HOME/juju-release-tools:$HOME/juju-ci-tools:$PATH"
source $(s3ci.py get $revision_build build-revision buildvars.bash)
set -x
export PATH
new_streams=$HOME/new-streams
testing=$new_streams/testing
agent_path=$testing/agent/revision-build-$revision_build
copy-agents.bash $HOME/streams/juju-dist/testing/tools $VERSION $agent_path
rb_stanzas=revision-build-$revision_build-paths.json
make-stanzas.bash $revision_build $VERSION $rb_stanzas
make-parallel-streams.bash $rb_stanzas $revision_build \
  $new_streams/testing-stanzas $testing
validate-streams.bash $testing
