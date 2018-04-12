// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/packaging/manager"
	"github.com/juju/utils/series"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
)

var (
	logger          = loggo.GetLogger("juju.mongo")
	mongoConfigPath = "/etc/default/mongodb"

	// JujuMongod24Path holds the default path to the legacy Juju
	// mongod.
	JujuMongod24Path = "/usr/lib/juju/bin/mongod"

	// This is NUMACTL package name for apt-get
	numaCtlPkg = "numactl"
)

// StorageEngine represents the storage used by mongo.
type StorageEngine string

const (
	// JujuMongoPackage is the mongo package Juju uses when
	// installing mongo.
	JujuMongoPackage = "juju-mongodb3.2"

	// JujuMongoToolsPackage is the mongo package Juju uses when
	// installing mongo tools to get mongodump etc.
	JujuMongoToolsPackage = "juju-mongo-tools3.2"

	// MMAPV1 is the default storage engine in mongo db up to 3.x
	MMAPV1 StorageEngine = "mmapv1"

	// WiredTiger is a storage type introduced in 3
	WiredTiger StorageEngine = "wiredTiger"

	// Upgrading is a special case where mongo is being upgraded.
	Upgrading StorageEngine = "Upgrading"
)

// Version represents the major.minor version of the runnig mongo.
type Version struct {
	Major         int
	Minor         int
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
	vParts := strings.SplitN(parts[0], ".", 3)

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
	if len(vParts) == 3 {
		version.Patch = vParts[2]
	}

	if version.Major == 2 && version.StorageEngine == WiredTiger {
		return Version{}, errors.Errorf("Version 2.x does not support Wired Tiger storage engine")
	}

	// This deserialises the special "Mongo Upgrading" version
	if version.Major == 0 && version.Minor == 0 {
		return Version{StorageEngine: Upgrading}, nil
	}

	return version, nil
}

// String serializes the version into a string.
func (v Version) String() string {
	s := fmt.Sprintf("%d.%d", v.Major, v.Minor)
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
	// MongoUpgrade represents a sepacial case where an upgrade is in
	// progress.
	MongoUpgrade = Version{Major: 0,
		Minor:         0,
		Patch:         "Upgrading",
		StorageEngine: Upgrading,
	}
)

// InstalledVersion returns the version of mongo installed.
// We look for a specific, known version supported by this Juju,
// and fall back to the original mongo 2.4.
func InstalledVersion() Version {
	mgoVersion := Mongo24
	if binariesAvailable(Mongo32wt, os.Stat) {
		mgoVersion = Mongo32wt
	}
	return mgoVersion
}

// binariesAvailable returns true if the binaries for the
// given Version of mongo are available.
func binariesAvailable(v Version, statFunc func(string) (os.FileInfo, error)) bool {
	var path string
	switch v {
	case Mongo24:
		// 2.4 has a fixed path.
		path = JujuMongod24Path
	default:
		path = JujuMongodPath(v)
	}
	if _, err := statFunc(path); err == nil {
		return true
	}
	return false
}

// WithAddresses represents an entity that has a set of
// addresses. e.g. a state Machine object
type WithAddresses interface {
	Addresses() []network.Address
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
// address by selecting it from the given addresses. If no addresses are
// available an empty string is returned.
func SelectPeerAddress(addrs []network.Address) string {
	// ScopeMachineLocal addresses are never suitable for mongo peers,
	// as each controller runs on a separate machine.
	const allowMachineLocal = false

	// The second bool result is ignored intentionally (we return an empty
	// string if no suitable address is available.)
	addr, _ := network.SelectControllerAddress(addrs, allowMachineLocal)
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
	switch version {
	case Mongo24:
		if _, err := stat(JujuMongod24Path); err == nil {
			return JujuMongod24Path, nil
		}

		path, err := lookPath("mongod")
		if err != nil {
			logger.Infof("could not find %v or mongod in $PATH", JujuMongod24Path)
			return "", err
		}
		return path, nil
	default:
		path := JujuMongodPath(version)
		var err error
		if _, err = stat(path); err == nil {
			return path, nil
		}
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
	MemoryProfileDefault = "default"
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
	logger.Infof(
		"Ensuring mongo server is running; data directory %s; port %d",
		args.DataDir, args.StatePort,
	)

	dbDir := filepath.Join(args.DataDir, "db")
	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return fmt.Errorf("cannot create mongo database directory: %v", err)
	}

	oplogSizeMB := args.OplogSize
	if oplogSizeMB == 0 {
		var err error
		if oplogSizeMB, err = defaultOplogSize(dbDir); err != nil {
			return err
		}
	}

	if err := installMongod(series.MustHostSeries(), args.SetNUMAControlPolicy); err != nil {
		// This isn't treated as fatal because the Juju MongoDB
		// package is likely to be already installed anyway. There
		// could just be a temporary issue with apt-get/yum/whatever
		// and we don't want this to stop jujud from starting.
		// (LP #1441904)
		logger.Errorf("cannot install/upgrade mongod (will proceed anyway): %v", err)
	}
	mgoVersion := InstalledVersion()
	mongoPath, err := Path(mgoVersion)
	if err != nil {
		return err
	}
	logVersion(mongoPath)

	if err := UpdateSSLKey(args.DataDir, args.Cert, args.PrivateKey); err != nil {
		return err
	}

	err = utils.AtomicWriteFile(sharedSecretPath(args.DataDir), []byte(args.SharedSecret), 0600)
	if err != nil {
		return fmt.Errorf("cannot write mongod shared secret: %v", err)
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
			return err
		}
	}

	svcConf := newConf(ConfigArgs{
		DataDir:       args.DataDir,
		DBDir:         dbDir,
		MongoPath:     mongoPath,
		Port:          args.StatePort,
		OplogSizeMB:   oplogSizeMB,
		WantNUMACtl:   args.SetNUMAControlPolicy,
		Version:       mgoVersion,
		Auth:          true,
		IPv6:          network.SupportsIPv6(),
		MemoryProfile: args.MemoryProfile,
	})
	svc, err := newService(ServiceName, svcConf)
	if err != nil {
		return err
	}
	installed, err := svc.Installed()
	if err != nil {
		return errors.Trace(err)
	}
	if installed {
		exists, err := svc.Exists()
		if err != nil {
			return errors.Trace(err)
		}
		if exists {
			logger.Debugf("mongo exists as expected")
			running, err := svc.Running()
			if err != nil {
				return errors.Trace(err)
			}
			if !running {
				return svc.Start()
			}
			return nil
		}
	}

	if err := svc.Stop(); err != nil {
		return errors.Annotatef(err, "failed to stop mongo")
	}
	if err := makeJournalDirs(dbDir); err != nil {
		return fmt.Errorf("error creating journal directories: %v", err)
	}
	if err := preallocOplog(dbDir, oplogSizeMB); err != nil {
		return fmt.Errorf("error creating oplog files: %v", err)
	}

	if err := service.InstallAndStart(svc); err != nil {
		return errors.Trace(err)
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
	certKey := cert + "\n" + privateKey
	err := utils.AtomicWriteFile(sslKeyPath(dataDir), []byte(certKey), 0600)
	return errors.Annotate(err, "cannot write SSL key")
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

func installPackage(pkg string, pacconfer config.PackagingConfigurer, pacman manager.PackageManager) error {
	// apply release targeting if needed.
	if pacconfer.IsCloudArchivePackage(pkg) {
		pkg = strings.Join(pacconfer.ApplyCloudArchiveTarget(pkg), " ")
	}

	return pacman.Install(pkg)
}

func installMongod(operatingsystem string, numaCtl bool) error {
	// fetch the packaging configuration manager for the current operating system.
	pacconfer, err := config.NewPackagingConfigurer(operatingsystem)
	if err != nil {
		return err
	}

	// fetch the package manager implementation for the current operating system.
	pacman, err := manager.NewPackageManager(operatingsystem)
	if err != nil {
		return err
	}

	// CentOS requires "epel-release" for the epel repo mongodb-server is in.
	if operatingsystem == "centos7" {
		// install epel-release
		if err := pacman.Install("epel-release"); err != nil {
			return err
		}
	}

	mongoPkgs, fallbackPkgs := packagesForSeries(operatingsystem)

	if numaCtl {
		logger.Infof("installing %v and %s", mongoPkgs, numaCtlPkg)
		if err = installPackage(numaCtlPkg, pacconfer, pacman); err != nil {
			return errors.Trace(err)
		}
	} else {
		logger.Infof("installing %v", mongoPkgs)
	}

	for i := range mongoPkgs {
		if err = installPackage(mongoPkgs[i], pacconfer, pacman); err != nil {
			break
		}
	}
	if err != nil && len(fallbackPkgs) == 0 {
		return errors.Trace(err)
	}
	if err != nil {
		logger.Errorf("installing mongo failed: %v", err)
		logger.Infof("will try fallback packages %v", fallbackPkgs)
		for i := range fallbackPkgs {
			if err = installPackage(fallbackPkgs[i], pacconfer, pacman); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Work around SELinux on centos7
	if operatingsystem == "centos7" {
		cmd := []string{"chcon", "-R", "-v", "-t", "mongod_var_lib_t", "/var/lib/juju/"}
		logger.Infof("running %s %v", cmd[0], cmd[1:])
		_, err = utils.RunCommand(cmd[0], cmd[1:]...)
		if err != nil {
			logger.Errorf("chcon failed to change file security context error %s", err)
			return err
		}

		cmd = []string{"semanage", "port", "-a", "-t", "mongod_port_t", "-p", "tcp", strconv.Itoa(controller.DefaultStatePort)}
		logger.Infof("running %s %v", cmd[0], cmd[1:])
		_, err = utils.RunCommand(cmd[0], cmd[1:]...)
		if err != nil {
			if !strings.Contains(err.Error(), "exit status 1") {
				logger.Errorf("semanage failed to provide access on port %d error %s", controller.DefaultStatePort, err)
				return err
			}
		}
	}

	return nil
}

// packagesForSeries returns the name of the mongo package for the series
// of the machine that it is going to be running on plus a fallback for
// options where the package is going to be ready eventually but might not
// yet be.
func packagesForSeries(series string) ([]string, []string) {
	switch series {
	case "precise", "centos7":
		return []string{"mongodb-server"}, []string{}
	case "trusty":
		return []string{"juju-mongodb"}, []string{}
	default:
		// xenial and onwards
		return []string{JujuMongoPackage, JujuMongoToolsPackage}, []string{}
	}
}

// DbDir returns the dir where mongo storage is.
func DbDir(dataDir string) string {
	return filepath.Join(dataDir, "db")
}
