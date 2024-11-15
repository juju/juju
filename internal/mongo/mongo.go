// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
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
	"github.com/juju/utils/v4"

	"github.com/juju/juju/core/base"
	"github.com/juju/juju/core/network"
	coreos "github.com/juju/juju/core/os"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/packaging"
	"github.com/juju/juju/internal/packaging/dependency"
	"github.com/juju/juju/internal/service/common"
	"github.com/juju/juju/internal/service/snap"
	"github.com/juju/juju/internal/service/systemd"
)

var logger = internallogger.GetLogger("juju.mongo")

// StorageEngine represents the storage used by mongo.
type StorageEngine string

const (
	// JujuDbSnap is the snap of MongoDB that Juju uses.
	JujuDbSnap = "juju-db"

	// WiredTiger is a storage type introduced in 3
	WiredTiger StorageEngine = "wiredTiger"
)

// JujuDbSnapMongodPath is the path that the juju-db snap
// makes mongod available at
var JujuDbSnapMongodPath = "/snap/bin/juju-db.mongod"

// WithAddresses represents an entity that has a set of
// addresses. e.g. a state Machine object
type WithAddresses interface {
	Addresses() network.SpaceAddresses
}

// IsMaster returns a boolean that represents whether the given
// machine's peer address is the primary mongo host for the replicaset
var IsMaster = isMaster

func isMaster(session *mgo.Session, obj WithAddresses) (bool, error) {
	addrs := obj.Addresses()

	masterHostPort, err := replicaset.MasterHostPort(session)

	// If the replica set has not been configured, then we
	// can have only one master and the caller must
	// be that master.
	if err == replicaset.ErrMasterNotConfigured {
		return true, nil
	}
	if err != nil {
		return false, err
	}

	masterAddr, _, err := net.SplitHostPort(masterHostPort)
	if err != nil {
		return false, err
	}

	for _, addr := range addrs {
		if addr.Value == masterAddr {
			return true, nil
		}
	}
	return false, nil
}

// SelectPeerAddress returns the address to use as the mongo replica set peer
// address by selecting it from the given addresses.
// If no addresses are available an empty string is returned.
func SelectPeerAddress(addrs network.ProviderAddresses) string {
	// The second bool result is ignored intentionally (we return an empty
	// string if no suitable address is available.)
	addr, _ := addrs.OneMatchingScope(network.ScopeMatchCloudLocal)
	return addr.Value
}

// GenerateSharedSecret generates a pseudo-random shared secret (keyfile)
// for use with Mongo replica sets.
func GenerateSharedSecret() (string, error) {
	// "A key’s length must be between 6 and 1024 characters and may
	// only contain characters in the base64 set."
	//   -- http://docs.mongodb.org/manual/tutorial/generate-key-file/
	buf := make([]byte, base64.StdEncoding.DecodedLen(1024))
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("cannot read random secret: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

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

// NewMemoryProfile returns a Memory Profile from the passed value.
func NewMemoryProfile(m string) (MemoryProfile, error) {
	mp := MemoryProfile(m)
	if err := mp.Validate(); err != nil {
		return MemoryProfile(""), err
	}
	return mp, nil
}

// MemoryProfile represents a type of meory configuration for Mongo.
type MemoryProfile string

// String returns a string representation of this profile value.
func (m MemoryProfile) String() string {
	return string(m)
}

func (m MemoryProfile) Validate() error {
	if m != MemoryProfileLow && m != MemoryProfileDefault {
		return errors.NotValidf("memory profile %q", m)
	}
	return nil
}

const (
	// MemoryProfileLow will use as little memory as possible in mongo.
	MemoryProfileLow MemoryProfile = "low"
	// MemoryProfileDefault will use mongo config ootb.
	MemoryProfileDefault MemoryProfile = "default"
)

// EnsureServerParams is a parameter struct for EnsureServer.
type EnsureServerParams struct {
	// APIPort is the port to connect to the api server.
	APIPort int

	// StatePort is the port to connect to the mongo server.
	StatePort int

	// Cert is the certificate.
	Cert string

	// PrivateKey is the certificate's private key.
	PrivateKey string

	// CAPrivateKey is the CA certificate's private key.
	CAPrivateKey string

	// SharedSecret is a secret shared between mongo servers.
	SharedSecret string

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

	// MemoryProfile determines which value is going to be used by
	// the cache and future memory tweaks.
	MemoryProfile MemoryProfile

	// The channel for installing the mongo snap in focal and later.
	JujuDBSnapChannel string
}

// EnsureServerInstalled ensures that the MongoDB server is installed,
// configured, and ready to run.
func EnsureServerInstalled(ctx context.Context, args EnsureServerParams) error {
	return ensureServer(ctx, args, mongoKernelTweaks)
}

func ensureServer(ctx context.Context, args EnsureServerParams, mongoKernelTweaks map[string]string) (err error) {
	tweakSysctlForMongo(mongoKernelTweaks)

	mongoDep := dependency.Mongo(args.JujuDBSnapChannel)
	if args.MongoDataDir == "" {
		args.MongoDataDir = dataPathForJujuDbSnap
	}
	if args.JujuDataDir == "" {
		args.JujuDataDir = dataPathForJuju
	}
	if args.ConfigDir == "" {
		args.ConfigDir = systemd.EtcSystemdDir
	}

	logger.Infof(ctx,
		"Ensuring mongo server is running; data directory %s; port %d",
		args.MongoDataDir, args.StatePort,
	)

	if err := setupDataDirectory(args); err != nil {
		return errors.Annotatef(err, "cannot set up data directory")
	}

	// TODO(wallyworld) - set up Numactl if requested in args.SetNUMAControlPolicy
	svc, err := mongoSnapService(args.JujuDataDir, args.ConfigDir, args.JujuDBSnapChannel)
	if err != nil {
		return errors.Annotatef(err, "cannot create mongo snap service")
	}

	hostBase, err := coreos.HostBase()
	if err != nil {
		return errors.Annotatef(err, "cannot get host base")
	}

	if err := installMongod(mongoDep, hostBase, svc); err != nil {
		return errors.Annotatef(err, "cannot install mongod")
	}

	finder := NewMongodFinder()
	mongoPath, err := finder.InstalledAt()
	if err != nil {
		return errors.Annotatef(err, "unable to find mongod install path")
	}
	logVersion(mongoPath)

	oplogSizeMB := args.OplogSize
	if oplogSizeMB == 0 {
		oplogSizeMB, err = defaultOplogSize(dbDir(args.MongoDataDir))
		if err != nil {
			return errors.Annotatef(err, "unable to calculate default oplog size")
		}
	}

	mongoArgs := generateConfig(oplogSizeMB, args)

	// Update snap configuration.
	// TODO(tsm): refactor out to service.Configure
	err = mongoArgs.writeConfig(configPath(args.MongoDataDir))
	if err != nil {
		return errors.Annotatef(err, "unable to write config")
	}
	if err := snap.SetSnapConfig(ServiceName, "configpath", configPath(args.MongoDataDir)); err != nil {
		return errors.Annotatef(err, "unable to set snap config")
	}

	// Update the systemd service configuration.
	if err := svc.ConfigOverride(); err != nil {
		return errors.Annotatef(err, "unable to update systemd service configuration")
	}

	// Ensure the mongo service is running, after we've installed and
	// configured it.
	// We do this in two retry loops. The outer loop, will try and start
	// the service repeatedly over the span of 5 minutes. The inner loop will
	// try and ensure that the service is running over the span of 10 seconds.
	// If the service is running, then it will return nil, causing the outer
	// loop to complete. If the service is not running, and the inner retry loop
	// has been exhausted, then the outer loop will attempt to start the service
	// again after a delay.
	// If the mongo service is not installed, then nothing we do here, will
	// cause the service to start. So we will just return the error.
	return retry.Call(retry.CallArgs{
		Func: func() error {
			if err := svc.Start(); err != nil {
				logger.Debugf(ctx, "cannot start mongo service: %v", err)
			}
			return ensureMongoServiceRunning(ctx, svc)
		},
		IsFatalError: func(err error) bool {
			// If the service is not installed, then we should attempt
			// to install it again, by bouncing.
			return errors.Cause(err) == ErrMongoServiceNotInstalled
		},
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf(ctx, "attempt %d to start mongo service: %v", attempt, err)
		},
		Stop:        ctx.Done(),
		Attempts:    -1,
		Delay:       10 * time.Second,
		MaxDelay:    1 * time.Minute,
		MaxDuration: time.Minute * 5,
		BackoffFunc: retry.DoubleDelay,
		Clock:       clock.WallClock,
	})
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
				return errors.Annotatef(ErrMongoServiceNotInstalled, "%s", err.Error())
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

	// TODO(fix): rather than copy, we should ln -s coz it could be changed later!!!
	if err := UpdateSSLKey(args.MongoDataDir, args.Cert, args.PrivateKey); err != nil {
		return errors.Trace(err)
	}

	err := utils.AtomicWriteFile(sharedSecretPath(args.MongoDataDir), []byte(args.SharedSecret), 0600)
	if err != nil {
		return errors.Annotatef(err, "cannot write mongod shared secret to %v", sharedSecretPath(args.MongoDataDir))
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

// UpdateSSLKey writes a new SSL key used by mongo to validate connections from Juju controller(s)
func UpdateSSLKey(dataDir, cert, privateKey string) error {
	err := utils.AtomicWriteFile(sslKeyPath(dataDir), []byte(GenerateSSLKey(cert, privateKey)), 0600)
	return errors.Annotate(err, "cannot write SSL key")
}

// GenerateSSLKey combines cert and private key to generate the ssl key - server.pem.
func GenerateSSLKey(cert, privateKey string) string {
	return cert + "\n" + privateKey
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

func mongoSnapService(dataDir, configDir, snapChannel string) (MongoSnapService, error) {
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
		Channel:            snapChannel,
		BackgroundServices: backgroundServices,
	})
	return svc, errors.Trace(err)
}

// Override for testing.
var installMongo = packaging.InstallDependency

func installMongod(mongoDep packaging.Dependency, hostBase base.Base, snapSvc MongoSnapService) error {
	// Do either a local snap install or a real install from the store.
	if snapSvc.IsLocal() {
		// Local snap.
		return snapSvc.Install()
	}
	// Store snap.
	return installMongo(mongoDep, hostBase)
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
