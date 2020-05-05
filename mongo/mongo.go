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
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/featureflag"
	"github.com/juju/loggo"
	"github.com/juju/os/series"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/packaging"
	"github.com/juju/juju/packaging/dependency"
	"github.com/juju/juju/service"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/snap"
)

var (
	logger          = loggo.GetLogger("juju.mongo")
	mongoConfigPath = "/etc/default/mongodb"

	// JujuMongod24Path holds the default path to the legacy Juju
	// mongod.
	JujuMongod24Path = "/usr/lib/juju/bin/mongod"

	// JujuMongod32Path holds the default path to juju-mongodb3.2
	JujuMongod32Path = "/usr/lib/juju/mongo3.2/bin/mongod"

	// MongodSystemPath is actually just the system path
	MongodSystemPath = "/usr/bin/mongod"

	// mininmumSystemMongoVersion is the minimum version we would allow to be used from /usr/bin/mongod.
	minimumSystemMongoVersion = Version{Major: 3, Minor: 4}
)

// StorageEngine represents the storage used by mongo.
type StorageEngine string

const (
	// JujuDbSnap is the snap of MongoDB that Juju uses.
	JujuDbSnap = "juju-db"

	// JujuDbSnapMongodPath is the path that the juju-db snap
	// makes mongod available at
	JujuDbSnapMongodPath = "/snap/bin/juju-db.mongod"

	// MMAPV1 is the default storage engine in mongo db up to 3.x
	MMAPV1 StorageEngine = "mmapv1"

	// WiredTiger is a storage type introduced in 3
	WiredTiger StorageEngine = "wiredTiger"

	// Upgrading is a special case where mongo is being upgraded.
	Upgrading StorageEngine = "Upgrading"

	// SnapTrack is the track to get the juju-db snap from
	SnapTrack = "4.0"

	// SnapRisk is which juju-db snap to use i.e. stable or edge.
	SnapRisk = "stable"
)

// Version represents the major.minor version of the running mongo.
type Version struct {
	Major         int
	Minor         int
	Point         int
	Patch         string // supports variants like 1-alpha
	StorageEngine StorageEngine
}

// NewerThan will return 1 if the passed version is older than
// v, 0 if they are equal (or ver is a special case such as
// Upgrading and -1 if ver is newer.
func (v Version) NewerThan(ver Version) int {
	if v == MongoUpgrade || ver == MongoUpgrade {
		return 0
	}
	if v.Major > ver.Major {
		return 1
	}
	if v.Major < ver.Major {
		return -1
	}
	if v.Minor > ver.Minor {
		return 1
	}
	if v.Minor < ver.Minor {
		return -1
	}
	if v.Point > ver.Point {
		return 1
	}
	if v.Point < ver.Point {
		return -1
	}
	return 0
}

// NewVersion returns a mongo Version parsing the passed version string
// or error if not possible.
// A valid version string is of the form:
// 1.2.patch/storage
// major and minor are positive integers, patch is a string containing
// any ascii character except / and storage is one of the above defined
// StorageEngine. Only major is mandatory.
// An alternative valid string is 0.0/Upgrading which represents that
// mongo is being upgraded.
func NewVersion(v string) (Version, error) {
	version := Version{}
	if v == "" {
		return Mongo24, nil
	}

	parts := strings.SplitN(v, "/", 2)
	switch len(parts) {
	case 0:
		return Version{}, errors.New("invalid version string")
	case 1:
		version.StorageEngine = MMAPV1
	case 2:
		switch StorageEngine(parts[1]) {
		case MMAPV1:
			version.StorageEngine = MMAPV1
		case WiredTiger:
			version.StorageEngine = WiredTiger
		case Upgrading:
			version.StorageEngine = Upgrading
		}
	}
	vParts := strings.SplitN(parts[0], ".", 4)

	if len(vParts) >= 1 {
		i, err := strconv.Atoi(vParts[0])
		if err != nil {
			return Version{}, errors.Annotate(err, "Invalid version string, major is not an int")
		}
		version.Major = i
	}
	if len(vParts) >= 2 {
		i, err := strconv.Atoi(vParts[1])
		if err != nil {
			return Version{}, errors.Annotate(err, "Invalid version string, minor is not an int")
		}
		version.Minor = i
	}
	if len(vParts) >= 3 {
		i, err := strconv.Atoi(vParts[2])
		if err != nil {
			return Version{}, errors.Annotate(err, "Invalid version string, point is not an int")
		}
		version.Point = i
	}
	if len(vParts) == 4 {
		version.Patch = vParts[3]
	}

	if version.Major == 2 && version.StorageEngine == WiredTiger {
		return Version{}, errors.Errorf("Version 2.x does not support Wired Tiger storage engine")
	}

	// This deserialises the special "Mongo Upgrading" version
	if version.Major == 0 && version.Minor == 0 && version.Point == 0 {
		return Version{StorageEngine: Upgrading}, nil
	}

	return version, nil
}

// String serializes the version into a string.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d", v.Major, v.Minor)
	if v.Point > 0 {
		s = fmt.Sprintf("%s.%d", s, v.Point)
	}
	if v.Patch != "" {
		s = fmt.Sprintf("%s.%s", s, v.Patch)
	}
	if v.StorageEngine != "" {
		s = fmt.Sprintf("%s/%s", s, v.StorageEngine)
	}
	return s
}

// JujuMongodPath returns the path for the mongod binary
// with the specified version.
func JujuMongodPath(v Version) string {
	return fmt.Sprintf("/usr/lib/juju/mongo%d.%d/bin/mongod", v.Major, v.Minor)
}

var (
	// Mongo24 represents juju-mongodb 2.4.x
	Mongo24 = Version{Major: 2,
		Minor:         4,
		Patch:         "",
		StorageEngine: MMAPV1,
	}
	// Mongo26 represents juju-mongodb26 2.6.x
	Mongo26 = Version{Major: 2,
		Minor:         6,
		Patch:         "",
		StorageEngine: MMAPV1,
	}
	// Mongo32wt represents juju-mongodb3 3.2.x with wiredTiger storage.
	Mongo32wt = Version{Major: 3,
		Minor:         2,
		Patch:         "",
		StorageEngine: WiredTiger,
	}
	// Mongo36wt represents 'mongodb-server-core' at version 3.6.x with WiredTiger
	Mongo36wt = Version{Major: 3,
		Minor:         6,
		Patch:         "",
		StorageEngine: WiredTiger,
	}
	// Mongo40wt represents 'mongodb' at version 4.0.x with WiredTiger
	Mongo40wt = Version{Major: 4,
		Minor:         0,
		Patch:         "",
		StorageEngine: WiredTiger,
	}
	// MongoUpgrade represents a sepacial case where an upgrade is in
	// progress.
	MongoUpgrade = Version{Major: 0,
		Minor:         0,
		Patch:         "Upgrading",
		StorageEngine: Upgrading,
	}
)

// WithAddresses represents an entity that has a set of
// addresses. e.g. a state Machine object
type WithAddresses interface {
	Addresses() network.SpaceAddresses
}

// IsMaster returns a boolean that represents whether the given
// machine's peer address is the primary mongo host for the replicaset
func IsMaster(session *mgo.Session, obj WithAddresses) (bool, error) {
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

// Path returns the executable path to be used to run mongod on this
// machine. If the juju-bundled version of mongo exists, it will return that
// path, otherwise it will return the command to run mongod from the path.
func Path(version Version) (string, error) {
	return mongoPath(version, os.Stat, exec.LookPath)
}

func mongoPath(
	version Version,
	stat func(string) (os.FileInfo, error),
	lookPath func(string) (string, error),
) (string, error) {
	// we don't want to match on patch so we remove it.
	if version.Major == 2 && version.Minor == 4 {
		if _, err := stat(JujuMongod24Path); err == nil {
			return JujuMongod24Path, nil
		}
		path, err := lookPath("mongod")
		if err != nil {
			logger.Infof("could not find %v or mongod in $PATH", JujuMongod24Path)
			return "", err
		}
		return path, nil
	}
	if version.Major == 3 && version.Minor == 6 {
		if _, err := stat(MongodSystemPath); err == nil {
			return MongodSystemPath, nil
		} else {
			return "", err
		}
	}
	path := JujuMongodPath(version)
	var err error
	if _, err = stat(path); err == nil {
		return path, nil
	}
	logger.Infof("could not find a suitable binary for %q", version)
	errMsg := fmt.Sprintf("no suitable binary for %q", version)
	return "", errors.New(errMsg)
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
func EnsureServer(args EnsureServerParams) (Version, error) {
	return ensureServer(args, mongoKernelTweaks)
}

func ensureServer(args EnsureServerParams, mongoKernelTweaks map[string]string) (Version, error) {
	var zeroVersion Version
	tweakSysctlForMongo(mongoKernelTweaks)

	hostSeries := series.MustHostSeries()
	mongoDep := dependency.Mongo(args.SetNUMAControlPolicy, args.JujuDBSnapChannel)
	usingMongoFromSnap := providesMongoAsSnap(mongoDep, hostSeries) || featureflag.Enabled(feature.MongoDbSnap)

	// TODO(tsm): clean up the args.DataDir handling. When using a snap, args.DataDir should be
	//            set earlier in the bootstrapping process. An extra variable is needed here because
	//            cloudconfig sends local snaps to /var/lib/juju/
	dataDir := args.DataDir
	if usingMongoFromSnap {
		if args.DataDir != dataPathForJujuDbSnap {
			logger.Warningf("overwriting args.dataDir (set to %v) to %v", args.DataDir, dataPathForJujuDbSnap)
			args.DataDir = dataPathForJujuDbSnap
		}
	}

	logger.Infof(
		"Ensuring mongo server is running; data directory %s; port %d",
		args.DataDir, args.StatePort,
	)

	setupDataDirectory(args, usingMongoFromSnap)

	if err := installMongod(mongoDep, hostSeries, dataDir); err != nil {
		// This isn't treated as fatal because the Juju MongoDB
		// package is likely to be already installed anyway. There
		// could just be a temporary issue with apt-get/yum/whatever
		// and we don't want this to stop jujud from starting.
		// (LP #1441904)
		logger.Errorf("cannot install/upgrade mongod (will proceed anyway): %v", err)
	}
	finder := NewMongodFinder()
	mongoPath, mongodVersion, err := finder.FindBest()
	if err != nil {
		return zeroVersion, errors.Trace(err)
	}
	logVersion(mongoPath)

	oplogSizeMB := args.OplogSize
	if oplogSizeMB == 0 {
		oplogSizeMB, err = defaultOplogSize(DbDir(args.DataDir))
		if err != nil {
			return zeroVersion, errors.Trace(err)
		}
	}

	// Disable the default mongodb installed by the mongodb-server package.
	// Only do this if the file doesn't exist already, so users can run
	// their own mongodb server if they wish to.
	if _, err := os.Stat(mongoConfigPath); os.IsNotExist(err) {
		err = utils.AtomicWriteFile(
			mongoConfigPath,
			[]byte("ENABLE_MONGODB=no"),
			0644,
		)
		if err != nil {
			return zeroVersion, errors.Trace(err)
		}
	}

	mongoArgs := generateConfig(mongoPath, oplogSizeMB, mongodVersion, usingMongoFromSnap, args)
	logger.Debugf("creating mongo service configuration for mongo version: %d.%d.%d-%s at %q",
		mongoArgs.Version.Major, mongoArgs.Version.Minor, mongoArgs.Version.Point, mongoArgs.Version.Patch, mongoArgs.MongoPath)

	svc, err := mongoArgs.asService(usingMongoFromSnap)
	if err != nil {
		return zeroVersion, errors.Trace(err)
	}

	// Update configuration if we are using mongo from snap (focal+ or
	// on an earlier series using the feature flag).
	// TODO(tsm): refactor out to service.Configure
	if usingMongoFromSnap {
		err = mongoArgs.writeConfig(configPath(args.DataDir))
		if err != nil {
			return zeroVersion, errors.Trace(err)
		}

		err := snap.SetSnapConfig(ServiceName, "configpath", configPath(args.DataDir))
		if err != nil {
			return zeroVersion, errors.Trace(err)
		}

		err = service.ManuallyRestart(svc)
		if err != nil {
			logger.Criticalf("unable to (re)start mongod service: %v", err)
			return zeroVersion, errors.Trace(err)
		}

		return mongodVersion, nil
	}

	// Installed tells us if there exists a service of the right name.
	installed, err := svc.Installed()
	if err != nil {
		return zeroVersion, errors.Trace(err)
	}
	if installed {
		// Exists() does a check against the contents of the service config file.
		// The return value is true iff the content is the same.
		exists, err := svc.Exists()
		if err != nil {
			return zeroVersion, errors.Trace(err)
		}
		if exists {
			logger.Debugf("mongo exists as expected")
			running, err := svc.Running()
			if err != nil {
				return zeroVersion, errors.Trace(err)
			}

			if !running {
				return mongodVersion, errors.Trace(svc.Start())
			}
			return mongodVersion, nil
		}
		logger.Debugf("updating mongo service configuration")
	}

	// We want to write or rewrite the contents of the service.
	// Stop is a no-op if the service doesn't exist or isn't running.
	if err := svc.Stop(); err != nil {
		return zeroVersion, errors.Annotatef(err, "failed to stop mongo")
	}
	dbDir := DbDir(args.DataDir)
	if err := makeJournalDirs(dbDir); err != nil {
		return zeroVersion, errors.Errorf("error creating journal directories: %v", err)
	}
	if err := preallocOplog(dbDir, oplogSizeMB); err != nil {
		return zeroVersion, errors.Errorf("error creating oplog files: %v", err)
	}

	if err := service.InstallAndStart(svc); err != nil {
		return zeroVersion, errors.Trace(err)
	}
	return mongodVersion, nil
}

func setupDataDirectory(args EnsureServerParams, usingMongoFromSnap bool) error {
	dbDir := DbDir(args.DataDir)
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

	if err := os.MkdirAll(logPath(dbDir, usingMongoFromSnap), 0644); err != nil {
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

func makeJournalDirs(dataDir string) error {
	journalDir := path.Join(dataDir, "journal")
	if err := os.MkdirAll(journalDir, 0700); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return err
	}

	// Manually create the prealloc files, since otherwise they get
	// created as 100M files. We create three files of 1MB each.
	prefix := filepath.Join(journalDir, "prealloc.")
	preallocSize := 1024 * 1024
	return preallocFiles(prefix, preallocSize, preallocSize, preallocSize)
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

func installMongod(mongoDep packaging.Dependency, hostSeries, dataDir string) error {
	// If we are not forcing a mongo snap via a feature flag, install the
	// package list (which may also include snaps for focal+) provided by
	// the mongo dependency for our series.
	if !featureflag.Enabled(feature.MongoDbSnap) {
		return packaging.InstallDependency(mongoDep, hostSeries)
	}

	snapName := ServiceName
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
	conf := common.Conf{Desc: ServiceName + " snap"}
	snapChannel := fmt.Sprintf("%s/%s", SnapTrack, SnapRisk)
	service, err := snap.NewService(snapName, ServiceName, conf, snap.Command, snapChannel, "", backgroundServices, prerequisites)
	if err != nil {
		return errors.Trace(err)
	}

	return service.Install()
}

// DbDir returns the dir where mongo storage is.
func DbDir(dataDir string) string {
	return filepath.Join(dataDir, "db")
}

// providesMongoAsSnap returns true if a mongo dependency provides mongo
// as a snap for the specified OS series.
func providesMongoAsSnap(mongoDep packaging.Dependency, series string) bool {
	pkgList, _ := mongoDep.PackageList(series)
	for _, pkg := range pkgList {
		if pkg.PackageManager == packaging.SnapPackageManager && pkg.Name == JujuDbSnap {
			return true
		}
	}
	return false
}
