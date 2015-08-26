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
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/replicaset"
	"github.com/juju/utils"
	"github.com/juju/utils/packaging/config"
	"github.com/juju/utils/packaging/manager"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/service"
	"github.com/juju/juju/version"
)

var (
	logger          = loggo.GetLogger("juju.mongo")
	mongoConfigPath = "/etc/default/mongodb"

	// JujuMongodPath holds the default path to the juju-specific
	// mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"

	// This is NUMACTL package name for apt-get
	numaCtlPkg = "numactl"
)

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

// SelectPeerAddress returns the address to use as the
// mongo replica set peer address by selecting it from the given addresses.
func SelectPeerAddress(addrs []network.Address) string {
	return network.SelectInternalAddress(addrs, false)
}

// SelectPeerHostPort returns the HostPort to use as the
// mongo replica set peer by selecting it from the given hostPorts.
func SelectPeerHostPort(hostPorts []network.HostPort) string {
	return network.SelectInternalHostPort(hostPorts, false)
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
func Path() (string, error) {
	if _, err := os.Stat(JujuMongodPath); err == nil {
		return JujuMongodPath, nil
	}

	path, err := exec.LookPath("mongod")
	if err != nil {
		logger.Infof("could not find %v or mongod in $PATH", JujuMongodPath)
		return "", err
	}
	return path, nil
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
}

// EnsureServer ensures that the MongoDB server is installed,
// configured, and ready to run.
//
// This method will remove old versions of the mongo init service as necessary
// before installing the new version.
//
// The namespace is a unique identifier to prevent multiple instances of mongo
// on this machine from colliding. This should be empty unless using
// the local provider.
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

	operatingsystem := version.Current.Series
	if err := installMongod(operatingsystem, args.SetNumaControlPolicy); err != nil {
		// This isn't treated as fatal because the Juju MongoDB
		// package is likely to be already installed anyway. There
		// could just be a temporary issue with apt-get/yum/whatever
		// and we don't want this to stop jujud from starting.
		// (LP #1441904)
		logger.Errorf("cannot install/upgrade mongod (will proceed anyway): %v", err)
	}
	mongoPath, err := Path()
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

	svcConf := newConf(args.DataDir, dbDir, mongoPath, args.StatePort, oplogSizeMB, args.SetNumaControlPolicy)
	svc, err := newService(ServiceName(args.Namespace), svcConf)
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

// UpdateSSLKey writes a new SSL key used by mongo to validate connections from Juju state server(s)
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

	mongoPkg := packageForSeries(operatingsystem)

	pkgs := []string{mongoPkg}
	if numaCtl {
		pkgs = []string{mongoPkg, numaCtlPkg}
		logger.Infof("installing %s and %s", mongoPkg, numaCtlPkg)
	} else {
		logger.Infof("installing %s", mongoPkg)
	}

	for i, _ := range pkgs {
		// apply release targeting if needed.
		if pacconfer.IsCloudArchivePackage(pkgs[i]) {
			pkgs[i] = strings.Join(pacconfer.ApplyCloudArchiveTarget(pkgs[i]), " ")
		}

		if err := pacman.Install(pkgs[i]); err != nil {
			return err
		}
	}

	// Work around SELinux on centos7
	if operatingsystem == "centos7" {
		cmd := []string{"chcon", "-R", "-v", "-t", "mongod_var_lib_t", "/var/lib/juju/"}
		logger.Infof("running %s %v", cmd[0], cmd[1:])
		_, err = utils.RunCommand(cmd[0], cmd[1:]...)
		if err != nil {
			logger.Infof("chcon error %s", err)
			logger.Infof("chcon error %s", err.Error())
			return err
		}

		cmd = []string{"semanage", "port", "-a", "-t", "mongod_port_t", "-p", "tcp", "37017"}
		logger.Infof("running %s %v", cmd[0], cmd[1:])
		_, err = utils.RunCommand(cmd[0], cmd[1:]...)
		if err != nil {
			if !strings.Contains(err.Error(), "exit status 1") {
				return err
			}
		}
	}

	return nil
}

// packageForSeries returns the name of the mongo package for the series
// of the machine that it is going to be running on.
func packageForSeries(series string) string {
	switch series {
	case "precise", "quantal", "raring", "saucy", "centos7":
		return "mongodb-server"
	default:
		// trusty and onwards
		return "juju-mongodb"
	}
}

// noauthCommand returns an os/exec.Cmd that may be executed to
// run mongod without security.
func noauthCommand(dataDir string, port int) (*exec.Cmd, error) {
	sslKeyFile := path.Join(dataDir, "server.pem")
	dbDir := filepath.Join(dataDir, "db")
	mongoPath, err := Path()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(mongoPath,
		"--noauth",
		"--dbpath", dbDir,
		"--sslOnNormalPorts",
		"--sslPEMKeyFile", sslKeyFile,
		"--sslPEMKeyPassword", "ignored",
		"--bind_ip", "127.0.0.1",
		"--port", fmt.Sprint(port),
		"--noprealloc",
		"--syslog",
		"--smallfiles",
		"--journal",
	)
	return cmd, nil
}
