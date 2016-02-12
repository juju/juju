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

	ServiceName    = "juju-db"
	serviceTimeout = 300 // 5 minutes

	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory.
	SharedSecretFile = "shared-secret"

	// ReplicaSetName is the name of the replica set that juju uses for its
	// controllers.
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
	Exists() (bool, error)
	Installed() (bool, error)
	Running() (bool, error)
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

// IsServiceInstalled returns whether the MongoDB init service
// configuration is present.
var IsServiceInstalled = isServiceInstalled

func isServiceInstalled() (bool, error) {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return false, errors.Trace(err)
	}
	return svc.Installed()
}

// RemoveService removes the mongoDB init service from this machine.
func RemoveService() error {
	svc, err := discoverService(ServiceName)
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

// StopService will stop mongodb service.
func StopService() error {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	return svc.Stop()
}

// StartService will start mongodb service.
func StartService() error {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	return svc.Start()
}

// ReStartService will stop and then start mongodb service.
func ReStartService() error {
	svc, err := discoverService(ServiceName)
	if err != nil {
		return errors.Trace(err)
	}
	if err := svc.Stop(); err != nil {
		return errors.Trace(err)
	}
	if err := svc.Start(); err != nil {
		return errors.Trace(err)
	}
	return nil
}

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, "server.pem")
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

// newConf returns the init system config for the mongo state service.
func newConf(dataDir, dbDir, mongoPath string, port, oplogSizeMB int, wantNumaCtl bool, version Version, auth bool) common.Conf {
	mongoCmd := mongoPath +

		" --dbpath " + utils.ShQuote(dbDir) +
		" --sslOnNormalPorts" +
		" --sslPEMKeyFile " + utils.ShQuote(sslKeyPath(dataDir)) +
		" --sslPEMKeyPassword ignored" +
		" --port " + fmt.Sprint(port) +
		" --syslog" +
		" --journal" +

		" --replSet " + ReplicaSetName +
		" --ipv6" +
		" --quiet" +
		" --oplogSize " + strconv.Itoa(oplogSizeMB)

	if auth {
		mongoCmd = mongoCmd +
			" --auth" +
			" --keyFile " + utils.ShQuote(sharedSecretPath(dataDir))
	} else {
		mongoCmd = mongoCmd +
			" --noauth"
	}
	// TODO(perrito666) implement a proper version comparision with <>
	// also make sure storageEngine is explicit every time it is possible.
	if version != Mongo30wt {
		mongoCmd = mongoCmd +
			" --noprealloc" +
			" --smallfiles"
	} else {
		mongoCmd = mongoCmd +
			" --storageEngine wiredTiger"
	}
	extraScript := ""
	if wantNumaCtl {
		extraScript = fmt.Sprintf(detectMultiNodeScript, multinodeVarName, multinodeVarName)
		mongoCmd = fmt.Sprintf(numaCtlWrap, multinodeVarName) + mongoCmd
	}
	conf := common.Conf{
		Desc: "juju state database",
		Limit: map[string]int{
			"nofile": maxFiles,
			"nproc":  maxProcs,
		},
		Timeout:     serviceTimeout,
		ExtraScript: extraScript,
		ExecStart:   mongoCmd,
	}
	return conf
}

// EnsureServiceInstalled is a convenience method to [re]create
// the mongo service.
func EnsureServiceInstalled(dataDir string, statePort, oplogSizeMB int, setNumaControlPolicy bool, version Version, auth bool) error {
	mongoPath, err := Path(version)
	if err != nil {
		return errors.Annotate(err, "cannot get mongo path")
	}

	dbDir := filepath.Join(dataDir, "db")

	if oplogSizeMB == 0 {
		var err error
		if oplogSizeMB, err = defaultOplogSize(dbDir); err != nil {
			return err
		}
	}

	svcConf := newConf(dataDir, dbDir, mongoPath, statePort, oplogSizeMB, setNumaControlPolicy, version, auth)
	svc, err := newService(ServiceName, svcConf)
	if err != nil {
		return errors.Trace(err)
	}
	if err := svc.Remove(); err != nil {
		return errors.Trace(err)
	}

	if err := svc.Install(); err != nil {
		return errors.Trace(err)
	}

	return nil
}
