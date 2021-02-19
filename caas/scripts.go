// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

var (
	// JujudStartUpSh is the start script for K8s controller and operator style agents.
	JujudStartUpSh = `
export JUJU_DATA_DIR=%[1]s
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/%[2]s

mkdir -p $JUJU_TOOLS_DIR
cp /opt/jujud $JUJU_TOOLS_DIR/jujud
%[3]s
`[1:]

	// ContainerAgentStartUpSh is the start script for in-pod style k8s agents.
	ContainerAgentStartUpSh = `
export JUJU_DATA_DIR=%[1]s
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/%[2]s

mkdir -p $JUJU_TOOLS_DIR
cp /opt/containeragent $JUJU_TOOLS_DIR/containeragent
# The in-pod style agent uses for hooks - hook tools are symlinks of jujuc.
cp /opt/jujuc $JUJU_TOOLS_DIR/jujuc
%[3]s
`[1:]
)
