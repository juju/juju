// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/mgo/v3"
	"github.com/juju/replicaset/v3"
	"github.com/juju/retry"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/packaging/dependency"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/service/snap"
)

var logger = internallogger.GetLogger("juju.mongo")

// StorageEngine represents the storage used by mongo.
type StorageEngine string

const (
	// WiredTiger is a storage type introduced in 3
	WiredTiger StorageEngine = "wiredTiger"
)

// JujuDbSnapMongodPath is the path that the juju-db snap
// makes mongod available at
var JujuDbSnapMongodPath = "/snap/bin/juju-db.mongod"

/*
Values set as per bug:
https://bugs.launchpad.net/juju/+bug/1656430
net.ipv4.tcp_max_syn_backlog = 4096
net.core.somaxconn = 16384
net.core.netdev_max_backlog = 1000
net.ipv4.tcp_fin_timeout = 30

Values set as per mongod recommendation (see syslog on default mongod run)
/sys/kernel/mm/transparent_hugepage/enabled 'always' > 'never'
/sys/kernel/mm/transparent_hugepage/defrag 'always' > 'never'
*/
// TODO(bootstrap): tweaks this to mongo OCI image.
var mongoKernelTweaks = map[string]string{
	"/sys/kernel/mm/transparent_hugepage/enabled": "never",
	"/sys/kernel/mm/transparent_hugepage/defrag":  "never",
	"/proc/sys/net/ipv4/tcp_max_syn_backlog":      "4096",
	"/proc/sys/net/core/somaxconn":                "16384",
	"/proc/sys/net/core/netdev_max_backlog":       "1000",
	"/proc/sys/net/ipv4/tcp_fin_timeout":          "30",
}

// EnsureServerParams is a parameter struct for EnsureServer.
type EnsureServerParams struct {
	// APIPort is the port to connect to the api server.
	APIPort int

	// Cert is the certificate.
	Cert string

	// PrivateKey is the certificate's private key.
	PrivateKey string

	// CAPrivateKey is the CA certificate's private key.
	CAPrivateKey string

	// SystemIdentity is the identity of the system.
	SystemIdentity string

	// MongoDataDir is the machine agent mongo data directory.
	MongoDataDir string

	// JujuDataDir is the directory where juju data is stored.
	JujuDataDir string

	// ConfigDir is where mongo config goes.
	ConfigDir string

	// Namespace is the machine agent's namespace, which is used to
	// generate a unique service name for Mongo.
	Namespace string

	// OplogSize is the size of the Mongo oplog.
	// If this is zero, then EnsureServer will
	// calculate a default size according to the
	// algorithm defined in Mongo.
	OplogSize int

	// SetNUMAControlPolicy preference - whether the user
	// wants to set the numa control policy when starting mongo.
	SetNUMAControlPolicy bool
}

// EnsureServerInstalled ensures that the MongoDB server is installed,
// configured, and ready to run.
func EnsureServerInstalled(ctx context.Context, args EnsureServerParams) error {
	return nil
}

const (
	// ErrMongoServiceNotInstalled is returned when the mongo service is not
	// installed.
	ErrMongoServiceNotInstalled = errors.ConstError("mongo service not installed")
	// ErrMongoServiceNotRunning is returned when the mongo service is not
	// running.
	ErrMongoServiceNotRunning = errors.ConstError("mongo service not running")
)

func ensureMongoServiceRunning(ctx context.Context, svc MongoSnapService) error {
	return retry.Call(retry.CallArgs{
		Func: func() error {
			running, err := svc.Running()
			if err != nil {
				// If the service is not installed, then we should attempt
				// to install it.
				return errors.Annotate(ErrMongoServiceNotInstalled, err.Error())
			}
			if running {
				return nil
			}
			return ErrMongoServiceNotRunning
		},
		Stop:     ctx.Done(),
		Attempts: 10,
		Delay:    1 * time.Second,
		Clock:    clock.WallClock,
	})
}

func setupDataDirectory(args EnsureServerParams) error {
	dbDir := dbDir(args.MongoDataDir)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return errors.Annotate(err, "cannot create mongo database directory")
	}

	if err := os.MkdirAll(logPath(dbDir), 0755); err != nil {
		return errors.Annotate(err, "cannot create mongodb logging directory")
	}

	return nil
}

func truncateAndWriteIfExists(procFile, value string) error {
	if _, err := os.Stat(procFile); os.IsNotExist(err) {
		logger.Debugf(context.TODO(), "%q does not exist, will not set %q", procFile, value)
		return errors.Errorf("%q does not exist, will not set %q", procFile, value)
	}
	f, err := os.OpenFile(procFile, os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return errors.Trace(err)
	}
	defer f.Close()
	_, err = f.WriteString(value)
	return errors.Trace(err)
}

func tweakSysctlForMongo(editables map[string]string) {
	for editableFile, value := range editables {
		if err := truncateAndWriteIfExists(editableFile, value); err != nil {
			logger.Errorf(context.TODO(), "could not set the value of %q to %q because of: %v\n", editableFile, value, err)
		}
	}
}

func logVersion(mongoPath string) {
	cmd := exec.Command(mongoPath, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Infof(context.TODO(), "failed to read the output from %s --version: %v", mongoPath, err)
		return
	}
	logger.Debugf(context.TODO(), "using mongod: %s --version:\n%s", mongoPath, output)
}

func mongoSnapService(dataDir, configDir string) (MongoSnapService, error) {
	jujuDbLocalSnapPattern := regexp.MustCompile(`juju-db_[0-9]+\.snap`)
	jujuDbLocalAssertsPattern := regexp.MustCompile(`juju-db_[0-9]+\.assert`)

	// If we're installing a local snap, then provide an absolute path
	// as a snap <name>. snap install <name> will then do the Right Thing (TM).
	snapDir := path.Join(dataDir, "snap")
	files, err := os.ReadDir(snapDir)

	var (
		snapPath        string
		snapAssertsPath string
	)
	if err == nil {
		for _, fullFileName := range files {
			fileName := fullFileName.Name()
			if jujuDbLocalSnapPattern.MatchString(fileName) {
				snapPath = path.Join(snapDir, fileName)
			}
			if jujuDbLocalAssertsPattern.MatchString(fileName) {
				snapAssertsPath = path.Join(snapDir, fileName)
			}
		}
	}

	backgroundServices := []snap.BackgroundService{
		{
			Name:            "daemon",
			EnableAtStartup: true,
		},
	}

	conf := common.Conf{
		Desc:  ServiceName + " snap",
		Limit: mongoULimits,
	}

	svc, err := newSnapService(snap.ServiceConfig{
		ServiceName:        ServiceName,
		SnapPath:           snapPath,
		SnapAssertsPath:    snapAssertsPath,
		Conf:               conf,
		SnapExecutable:     snap.Command,
		ConfigDir:          configDir,
		BackgroundServices: backgroundServices,
	})
	return svc, errors.Trace(err)
}

// Override for testing.
var installMongo = dependency.InstallMongo

func installMongod(snapSvc MongoSnapService) error {
	// Do either a local snap install or a real install from the store.
	if snapSvc.IsLocal() {
		// Local snap.
		return snapSvc.Install()
	}
	// Store snap.
	return installMongo()
}

// dbDir returns the dir where mongo storage is.
func dbDir(dataDir string) string {
	return filepath.Join(dataDir, "db")
}

// MongoSnapService represents a mongo snap.
type MongoSnapService interface {
	Exists() (bool, error)
	Installed() (bool, error)
	Running() (bool, error)
	ConfigOverride() error
	Name() string
	IsLocal() bool
	Start() error
	Restart() error
	Install() error
}

var newSnapService = func(config snap.ServiceConfig) (MongoSnapService, error) {
	return snap.NewService(config)
}

// CurrentReplicasetConfig is overridden in tests.
var CurrentReplicasetConfig = func(session *mgo.Session) (*replicaset.Config, error) {
	return replicaset.CurrentConfig(session)
}
