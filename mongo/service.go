// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"fmt"
	"path/filepath"
	"strconv"

	"github.com/juju/errors"
	"github.com/juju/utils"

	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
)

const (
	maxFiles = 65000
	maxProcs = 20000

	serviceName = "juju-db"

	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory.
	SharedSecretFile = "shared-secret"

	// ReplicaSetName is the name of the replica set that juju uses for its
	// state servers.
	ReplicaSetName = "juju"
)

var (
	// This is the name of an environment variable that we use in the
	// init system conf file when mongo NUMA support is used.
	multinodeVarName = "MULTI_NODE"
	// This value will be used to wrap desired mongo cmd in numactl if wanted/needed
	numaCtlWrap = "$%v"
	// Extra shell script fragment for init script template.
	// This determines if we are dealing with multi-node environment
	detectMultiNodeScript = `%v=""
if [ $(find /sys/devices/system/node/ -maxdepth 1 -mindepth 1 -type d -name node\* | wc -l ) -gt 1 ]
then
    %v=" numactl --interleave=all "
    # Ensure sysctl turns off zone_reclaim_mode if not already set
    (grep -q vm.zone_reclaim_mode /etc/sysctl.conf || echo vm.zone_reclaim_mode = 0 >> /etc/sysctl.conf) && sysctl -p
fi
`
)

type mongoService interface {
	Exists() bool
	Running() bool
	Start() error
	Stop() error
	Install() error
	Remove() error
}

var newService = func(name string, conf common.Conf) (mongoService, error) {
	return service.DiscoverService(name, conf)
}

var discoverService = func(name string) (mongoService, error) {
	return service.DiscoverService(name, common.Conf{})
}

// RemoveService removes the mongoDB init service from this machine.
func RemoveService(namespace string) error {
	svc, err := discoverService(ServiceName(namespace))
	if err != nil {
		return errors.Trace(err)
	}
	if err := svc.Stop(); err != nil {
		return errors.Trace(err)
	}
	if err := svc.Remove(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// ServiceName returns the name of the init service config for mongo using
// the given namespace.
func ServiceName(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s-%s", serviceName, namespace)
	}
	return serviceName
}

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, "server.pem")
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

// mongodOptions defines how mongod command line arguments should be
// constructed.
type mongodOptions struct {
	dataDir     string
	dbDir       string
	logFile     string
	mongoPath   string
	port        int
	oplogSizeMB int
	wantNumaCtl bool
}

// newServiceConf returns the init system config for the mongo state service.
func newServiceConf(opts *mongodOptions) common.Conf {
	mongoCmd := opts.mongoPath + " --auth" +
		" --dbpath=" + utils.ShQuote(opts.dbDir) +
		" --sslOnNormalPorts" +
		" --sslPEMKeyFile " + utils.ShQuote(sslKeyPath(opts.dataDir)) +
		" --sslPEMKeyPassword ignored" +
		" --port " + fmt.Sprint(opts.port) +
		" --noprealloc" +
		" --logpath " + utils.ShQuote(opts.logFile) +
		" --smallfiles" +
		" --journal" +
		" --keyFile " + utils.ShQuote(sharedSecretPath(opts.dataDir)) +
		" --replSet " + ReplicaSetName +
		" --ipv6 " +
		" --oplogSize " + strconv.Itoa(opts.oplogSizeMB)
	extraScript := ""
	if opts.wantNumaCtl {
		extraScript = fmt.Sprintf(detectMultiNodeScript, multinodeVarName, multinodeVarName)
		mongoCmd = fmt.Sprintf(numaCtlWrap, multinodeVarName) + mongoCmd
	}
	return common.Conf{
		Desc: "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxFiles, maxFiles),
			"nproc":  fmt.Sprintf("%d %d", maxProcs, maxProcs),
		},
		ExtraScript: extraScript,
		ExecStart:   mongoCmd,
	}
}
