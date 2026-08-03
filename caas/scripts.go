// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

var (
	// ModelOperatorStartUpSh is the start script for CAAS model operator pods.
	// It detects whether the image provides the current jujuagentd binary or
	// only the legacy jujud binary (pre-rename), and invokes whichever is
	// present. This allows a controller that has been upgraded to continue
	// managing model operators whose pods are still running an older image
	// (i.e. models that have not yet been upgraded). Once the model is
	// upgraded the pod image is refreshed and jujuagentd is used directly.
	//
	// Format args:
	//   %[1]s - JUJU_DATA_DIR (e.g. /var/lib/juju)
	//   %[2]s - tools subdirectory name (always "tools")
	//   %[3]s - exec command using jujuagentd (new images)
	//   %[4]s - exec command using jujud    (old images)
	ModelOperatorStartUpSh = `
export JUJU_DATA_DIR=%[1]s
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/%[2]s

mkdir -p $JUJU_TOOLS_DIR
if [ -x /opt/jujuagentd ]; then
    cp /opt/jujuagentd $JUJU_TOOLS_DIR/jujuagentd
    %[3]s
else
    cp /opt/jujud $JUJU_TOOLS_DIR/jujud
    %[4]s
fi
`[1:]

	// APIServerStartUpSh is the start script for the "api-server" container
	// in the controller pod (Pebble running jujud).
	APIServerStartUpSh = `
export JUJU_DATA_DIR=%[1]s
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/tools

mkdir -p $JUJU_TOOLS_DIR
cp /opt/jujuagentd $JUJU_TOOLS_DIR/jujuagentd

%[2]s

mkdir -p /var/lib/pebble/default/layers
cat > /var/lib/pebble/default/layers/001-jujuagentd.yaml <<EOF
%[3]s
EOF

exec /opt/pebble run --http :%[4]s --verbose
`[1:]
)
