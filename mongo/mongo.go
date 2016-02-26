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

	environs "github.com/juju/juju/environs/config"
	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
)

var (
	logger          = loggo.GetLogger("juju.mongo")
	mongoConfigPath = "/etc/default/mongodb"

	// JujuMongodPath holds the default path to the juju-specific
	// mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"
	// JujuMongod26Path holds the default path for the transitional
	// mongo 2.6 to be installed to upgrade to 3.
	JujuMongod26Path = "/usr/lib/juju/mongo2.6/bin/mongod"
	// JujuMongod30Path holds the default path for mongo 3.
	JujuMongod30Path = "/usr/lib/juju/mongo3/bin/mongod"

	// This is NUMACTL package name for apt-get
	numaCtlPkg = "numactl"
)

// StorageEngine represents the storage used by mongo.
type StorageEngine string

const (
	// MMAPV2 is the default storage engine in mongo db up to 3.x
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
	if len(parts) == 0 {
		return Version{}, errors.New("invalid version string")
	}
	if len(parts) == 2 {
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
	// Mongo30 represents juju-mongodb3 3.x.x
	Mongo30 = Version{Major: 3,
		Minor:         0,
		Patch:         "",
		StorageEngine: MMAPV1,
	}
	// Mongo30wt represents juju-mongodb3 3.x.x with wiredTiger storage.
	Mongo30wt = Version{Major: 3,
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

// BinariesAvailable returns true if the binaries for the
// given Version of mongo are available.
func BinariesAvailable(v Version) bool {
	var path string
	switch v {
	case Mongo24:
		path = JujuMongodPath

	case Mongo26:
		path = JujuMongod26Path
	case Mongo30, Mongo30wt:
		path = JujuMongod30Path
	default:
		return false
	}
	if _, err := os.Stat(path); err == nil {
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
	logger.Debugf("selecting mongo peer address from %+v", addrs)
	// ScopeMachineLocal addresses are OK if we can't pick by space, also the
	// second bool return is ignored intentionally.
	addr, _ := network.SelectControllerAddress(addrs, true)
	return addr.Value
}

// SelectPeerHostPort returns the HostPort to use as the mongo replica set peer
// by selecting it from the given hostPorts.
func SelectPeerHostPort(hostPorts []network.HostPort) string {
	logger.Debugf("selecting mongo peer hostPort from %+v", hostPorts)
	// ScopeMachineLocal addresses are OK if we can't pick by space.
	return network.SelectControllerHostPort(hostPorts, true)
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
	noVersion := Version{}
	switch version {
	case Mongo24, noVersion:
		if _, err := os.Stat(JujuMongodPath); err == nil {
			return JujuMongodPath, nil
		}

		path, err := exec.LookPath("mongod")
		if err != nil {
			logger.Infof("could not find %v or mongod in $PATH", JujuMongodPath)
			return "", err
		}
		return path, nil

	case Mongo26:
		var err error
		if _, err = os.Stat(JujuMongod26Path); err == nil {
			return JujuMongod26Path, nil
		}
		logger.Infof("could not find %q ", JujuMongod26Path)
		return "", err

	case Mongo30, Mongo30wt:
		var err error
		if _, err = os.Stat(JujuMongod30Path); err == nil {
			return JujuMongod30Path, nil
		}
		logger.Infof("could not find %q", JujuMongod30Path)
		return "", err
	}

	logger.Infof("could not find a suitable binary for %q", version)
	errMsg := fmt.Sprintf("no suitable binary for %q", version)
	return "", errors.New(errMsg)

}

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

	// SetNumaControlPolicy preference - whether the user
	// wants to set the numa control policy when starting mongo.
	SetNumaControlPolicy bool

	// Version is the mongod version to be used.
	Version Version
}

// EnsureServer ensures that the MongoDB server is installed,
// configured, and ready to run.
//
// This method will remove old versions of the mongo init service as necessary
// before installing the new version.
func EnsureServer(args EnsureServerParams) error {
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

	operatingsystem := series.HostSeries()
	if err := installMongod(operatingsystem, args.SetNumaControlPolicy); err != nil {
		// This isn't treated as fatal because the Juju MongoDB
		// package is likely to be already installed anyway. There
		// could just be a temporary issue with apt-get/yum/whatever
		// and we don't want this to stop jujud from starting.
		// (LP #1441904)
		logger.Errorf("cannot install/upgrade mongod (will proceed anyway): %v", err)
	}
	mongoPath, err := Path(args.Version)
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

	svcConf := newConf(args.DataDir, dbDir, mongoPath, args.StatePort, oplogSizeMB, args.SetNumaControlPolicy, args.Version, true)
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

	// Only Quantal requires the PPA.
	if operatingsystem == "quantal" {
		// install python-software-properties:
		if err := pacman.InstallPrerequisite(); err != nil {
			return err
		}
		if err := pacman.AddRepository("ppa:juju/stable"); err != nil {
			return err
		}
	}
	// CentOS requires "epel-release" for the epel repo mongodb-server is in.
	if operatingsystem == "centos7" {
		// install epel-release
		if err := pacman.Install("epel-release"); err != nil {
			return err
		}
	}

	mongoPkgs := packagesForSeries(operatingsystem)

	pkgs := mongoPkgs
	if numaCtl {
		pkgs = append(mongoPkgs, numaCtlPkg)
		logger.Infof("installing %v and %s", mongoPkgs, numaCtlPkg)
	} else {
		logger.Infof("installing %v", mongoPkgs)
	}

	for i := range pkgs {
		// apply release targeting if needed.
		if pacconfer.IsCloudArchivePackage(pkgs[i]) {
			pkgs[i] = strings.Join(pacconfer.ApplyCloudArchiveTarget(pkgs[i]), " ")
		}

		if err := pacman.Install(pkgs[i]); err != nil {
			return err
		}
	}
	optionals := optionalPackagesForSeries(operatingsystem)
	for i := range optionals {
		// apply release targeting if needed.
		if pacconfer.IsCloudArchivePackage(optionals[i]) {
			optionals[i] = strings.Join(pacconfer.ApplyCloudArchiveTarget(optionals[i]), " ")
		}

		if err := pacman.Install(optionals[i]); err != nil {
			logger.Errorf("could not install package %q: %v", optionals[i], err)
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

		cmd = []string{"semanage", "port", "-a", "-t", "mongod_port_t", "-p", "tcp", strconv.Itoa(environs.DefaultStatePort)}
		logger.Infof("running %s %v", cmd[0], cmd[1:])
		_, err = utils.RunCommand(cmd[0], cmd[1:]...)
		if err != nil {
			if !strings.Contains(err.Error(), "exit status 1") {
				logger.Errorf("semanage failed to provide access on port %d error %s", environs.DefaultStatePort, err)
				return err
			}
		}
	}

	return nil
}

// packageForSeries returns the name of the mongo package for the series
// of the machine that it is going to be running on.
func packagesForSeries(series string) []string {
	switch series {
	case "precise", "quantal", "raring", "saucy", "centos7":
		return []string{"mongodb-server"}
	default:
		// trusty and onwards
		return []string{"juju-mongodb"}
	}
}

func optionalPackagesForSeries(series string) []string {
	switch series {
	case "precise", "quantal", "raring", "saucy", "centos7":
		return []string{}
	default:
		// TODO(perrito666) when the packages are ready, this should be
		// "juju-mongodb2.6", "juju-mongodb3"
		return []string{}
	}
}

// DbDir returns the dir where mongo storage is.
func DbDir(dataDir string) string {
	return filepath.Join(dataDir, "db")
}

// noauthCommand returns an os/exec.Cmd that may be executed to
// run mongod without security.
func noauthCommand(dataDir string, port int, version Version) (*exec.Cmd, error) {
	sslKeyFile := path.Join(dataDir, "server.pem")
	dbDir := DbDir(dataDir)
	// Make this smarter, to guess mongo version.
	mongoPath, err := Path(version)
	if err != nil {
		return nil, err
	}

	args := []string{
		"--noauth",
		"--dbpath", dbDir,
		"--sslOnNormalPorts",
		"--sslPEMKeyFile", sslKeyFile,
		"--sslPEMKeyPassword", "ignored",
		"--bind_ip", "127.0.0.1",
		"--port", fmt.Sprint(port),
		"--syslog",
		"--journal",
		"--quiet",
	}
	if version == Mongo30wt {
		args = append(args, "--storageEngine", "wiredTiger")

	} else {
		args = append(args, "--noprealloc", "--smallfiles")
	}
	if version == Mongo30 {
		args = append(args, "--storageEngine", "mmapv1")
	}

	cmd := exec.Command(mongoPath, args...)

	return cmd, nil
}

// ReplicaSetInformation holds information about replicaset
// components.
type ReplicaSetInformation struct {
	Master  replicaset.Member
	Members []replicaset.Member
	Config  replicaset.Config
}

// ReplicaSetInfo returns information describing the replicaset members
// and configuration
func ReplicaSetInfo(session *mgo.Session) (ReplicaSetInformation, error) {
	return ReplicaSetInformation{}, nil
}
