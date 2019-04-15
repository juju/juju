// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

var (
	// JujudStartUpSh is the exec script for CAAS controller.
	JujudStartUpSh = `

export JUJU_HOME=%[1]s
export JUJU_TOOLS_DIR=%[2]s

mkdir -p $JUJU_TOOLS_DIR
cp /opt/jujud $JUJU_TOOLS_DIR/jujud

%[3]s
`[1:]
)
