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

	// MongoStartupShTemplate is used to generate the start script for mongodb.
	MongoStartupShTemplate = `
args="%[1]s"
ipv6Disabled=$(sysctl net.ipv6.conf.all.disable_ipv6 -n)
if [ $ipv6Disabled -eq 0 ]; then
  args="${args} --ipv6"
fi
while [ ! -f "%[2]s" ]; do
  echo "Waiting for %[2]s to be created..."
  sleep 1
done
exec mongod ${args}
`[1:]
)
