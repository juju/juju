// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mgo/v2"
	"github.com/juju/os/v2/series"
	"github.com/juju/replicaset/v2"
	"github.com/juju/utils/v2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/packaging"
	"github.com/juju/juju/packaging/dependency"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/snap"
	"github.com/juju/juju/service/systemd"
)

var logger = loggo.GetLogger("juju.mongo")

// StorageEngine represents the storage used by mongo.
type StorageEngine string

const (
	// JujuDbSnap is the snap of MongoDB that Juju uses.
	JujuDbSnap = "juju-db"

	// WiredTiger is a storage type introduced in 3
	WiredTiger StorageEngine = "wiredTiger"

	// SnapTrack is the track to get the juju-db snap from
	SnapTrack = "4.0"

	// SnapRisk is which juju-db snap to use i.e. stable or edge.
	SnapRisk = "stable"
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

	// DataDir is the machine agent data directory.
	DataDir string

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

// EnsureServer ensures that the MongoDB server is installed,
// configured, and ready to run.
//
// This method will remove old versions of the mongo init service as necessary
// before installing the new version.
func EnsureServer(args EnsureServerParams) error {
	return ensureServer(args, mongoKernelTweaks)
}

func ensureServer(args EnsureServerParams, mongoKernelTweaks map[string]string) error {
	tweakSysctlForMongo(mongoKernelTweaks)

	hostSeries, err := series.HostSeries()
	if err != nil {
		return errors.Trace(err)
	}

	// We may have upgraded from 2.9 using an earlier
	// version on mongo so we need to keep using that.
	err = maybeUseLegacyMongo(args, &OSSearchTools{})
	if err != nil && !errors.IsNotFound(err) {
		return errors.Annotate(err, "checking legacy mongo")
	}
	if err == nil {
		return nil
	}

	mongoDep := dependency.Mongo(args.JujuDBSnapChannel)
	if args.DataDir == "" {
		args.DataDir = dataPathForJujuDbSnap
	}
	if args.ConfigDir == "" {
		args.ConfigDir = systemd.EtcSystemdDir
	}

	logger.Infof(
		"Ensuring mongo server is running; data directory %s; port %d",
		args.DataDir, args.StatePort,
	)

	if err := setupDataDirectory(args); err != nil {
		return errors.Trace(err)
	}

	// TODO(wallyworld) - set up Numactl if requested in args.SetNUMAControlPolicy
	svc, err := installMongod(mongoDep, hostSeries, args.DataDir, args.ConfigDir)
	if err != nil {
		return errors.Trace(err)
	}

	finder := NewMongodFinder()
	mongoPath, err := finder.InstalledAt()
	if err != nil {
		return errors.Trace(err)
	}
	logVersion(mongoPath)

	oplogSizeMB := args.OplogSize
	if oplogSizeMB == 0 {
		oplogSizeMB, err = defaultOplogSize(dbDir(args.DataDir))
		if err != nil {
			return errors.Trace(err)
		}
	}

	mongoArgs := generateConfig(oplogSizeMB, args)

	// Update snap configuration.
	// TODO(tsm): refactor out to service.Configure
	err = mongoArgs.writeConfig(configPath(args.DataDir))
	if err != nil {
		return errors.Trace(err)
	}
	if err := snap.SetSnapConfig(ServiceName, "configpath", configPath(args.DataDir)); err != nil {
		return errors.Trace(err)
	}

	// Update the systemd service configuration.
	if err := svc.ConfigOverride(); err != nil {
		return errors.Trace(err)
	}

	err = svc.Restart()
	if err != nil {
		logger.Criticalf("unable to (re)start mongod snap service: %v", err)
		return errors.Trace(err)
	}

	return nil
}

func setupDataDirectory(args EnsureServerParams) error {
	dbDir := dbDir(args.DataDir)
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return errors.Annotate(err, "cannot create mongo database directory")
	}

	// TODO(fix): rather than copy, we should ln -s coz it could be changed later!!!
	if err := UpdateSSLKey(args.DataDir, args.Cert, args.PrivateKey); err != nil {
		return errors.Trace(err)
	}

	err := utils.AtomicWriteFile(sharedSecretPath(args.DataDir), []byte(args.SharedSecret), 0600)
	if err != nil {
		return errors.Annotatef(err, "cannot write mongod shared secret to %v", sharedSecretPath(args.DataDir))
	}

	if err := os.MkdirAll(logPath(dbDir), 0755); err != nil {
		return errors.Annotate(err, "cannot create mongodb logging directory")
	}

	return nil
}

func truncateAndWriteIfExists(procFile, value string) error {
	if _, err := os.Stat(procFile); os.IsNotExist(err) {
		logger.Debugf("%q does not exist, will not set %q", procFile, value)
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
			logger.Errorf("could not set the value of %q to %q because of: %v\n", editableFile, value, err)
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
		logger.Infof("failed to read the output from %s --version: %v", mongoPath, err)
		return
	}
	logger.Debugf("using mongod: %s --version: %q", mongoPath, output)
}

// Override for testing.
var installMongo = packaging.InstallDependency

func installMongod(mongoDep packaging.Dependency, hostSeries, dataDir, configDir string) (*snap.Service, error) {
	snapName := JujuDbSnap
	jujuDbLocalSnapPattern := regexp.MustCompile(`juju-db_[0-9]+\.snap`)

	// If we're installing a local snap, then provide an absolute path
	// as a snap <name>. snap install <name> will then do the Right Thing (TM).
	files, err := ioutil.ReadDir(path.Join(dataDir, "snap"))
	if err == nil {
		for _, fullFileName := range files {
			_, fileName := path.Split(fullFileName.Name())
			if jujuDbLocalSnapPattern.MatchString(fileName) {
				snapName = fullFileName.Name()
			}
		}
	}

	prerequisites := []snap.App{snap.NewApp("core")}
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
	snapChannel := fmt.Sprintf("%s/%s", SnapTrack, SnapRisk)
	snapSvc, err := snap.NewService(
		snapName, ServiceName, conf, snap.Command, configDir, snapChannel, "",
		backgroundServices, prerequisites)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Do either a local snap install or a real install from the store.
	if snapName == ServiceName {
		// Package.
		err = installMongo(mongoDep, hostSeries)
	} else {
		// Local snap.
		err = snapSvc.Install()
	}
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &snapSvc, nil
}

// dbDir returns the dir where mongo storage is.
func dbDir(dataDir string) string {
	return filepath.Join(dataDir, "db")
}
