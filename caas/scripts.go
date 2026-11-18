// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

var (
	// ModelOperatorStartUpSh is the start script for CAAS model operator pods.
	// It selects the FHS layout shipped by the chiselled rock before falling
	// back to the /opt layout of the Docker image. It detects whether the
	// image provides the current jujuagentd binary or only the legacy jujud
	// binary (pre-rename), and invokes whichever is present. This allows a
	// controller that has been upgraded to continue managing model operators
	// whose pods are still running an older image (i.e. models that have not
	// yet been upgraded). Once the model is upgraded the pod image is
	// refreshed and jujuagentd is used directly.
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
# Temporary fallbacks to keep Docker image working, will be removed together
# with the Docker image.
if [ -x /usr/bin/jujuagentd ]; then
    cp /usr/bin/jujuagentd $JUJU_TOOLS_DIR/jujuagentd
    %[3]s
elif [ -x /opt/jujuagentd ]; then
    cp /opt/jujuagentd $JUJU_TOOLS_DIR/jujuagentd
    %[3]s
elif [ -x /usr/bin/jujud ]; then
    cp /usr/bin/jujud $JUJU_TOOLS_DIR/jujud
    %[4]s
else
    cp /opt/jujud $JUJU_TOOLS_DIR/jujud
    %[4]s
fi
`[1:]

	// APIServerStartUpSh is the start script for the "api-server" container
	// in the controller pod (Pebble running jujud and jujuagentd). The
	// binaries and Pebble are selected FHS-first, with the /opt layout of
	// the Docker image as the compatibility fallback.
	APIServerStartUpSh = `
export JUJU_DATA_DIR=%[1]s
export JUJU_TOOLS_DIR=$JUJU_DATA_DIR/tools
export JUJU_CONTROLLER_DIR=%[1]s/controller
export JUJU_BOOTSTRAP_PARAMS_PATH=%[1]s/bootstrap-params

# Temporary fallbacks (jujud, jujuagentd and pebble) to keep Docker image
# working, will be removed together with the Docker image.
jujud=/usr/bin/jujud
if [ ! -x "$jujud" ]; then
    jujud=/opt/jujud
fi
jujuagentd=/usr/bin/jujuagentd
if [ ! -x "$jujuagentd" ]; then
    jujuagentd=/opt/jujuagentd
fi
pebble=/usr/lib/juju/pebble
if [ ! -x "$pebble" ]; then
    pebble=/opt/pebble
fi

mkdir -p $JUJU_TOOLS_DIR
cp "$jujud" $JUJU_TOOLS_DIR/jujud
cp "$jujuagentd" $JUJU_TOOLS_DIR/jujuagentd

%[2]s

mkdir -p /var/lib/pebble/default/layers
cat > /var/lib/pebble/default/layers/001-controller.yaml <<'EOF'
%[3]s
EOF

exec "$pebble" run --http :%[4]s --verbose
`[1:]

	// ContainerAgentStartUpSh is the start script for the unit-agent init
	// container. It selects the binary from the FHS layout shipped by the
	// chiselled rock before falling back to the /opt layout of the
	// Docker image. The init arguments are passed through unchanged as
	// positional parameters.
	ContainerAgentStartUpSh = `
# Temporary fallback to keep Docker image working, will be removed together
# with the Docker image.
if [ -x /usr/bin/containeragent ]; then
    exec /usr/bin/containeragent "$@"
fi
exec /opt/containeragent "$@"
`[1:]
)
