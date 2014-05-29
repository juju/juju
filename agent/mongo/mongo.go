// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/loggo"
	"labix.org/v2/mgo"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
	"launchpad.net/juju-core/utils/apt"
	"launchpad.net/juju-core/version"
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
	logger          = loggo.GetLogger("juju.agent.mongo")
	mongoConfigPath = "/etc/default/mongodb"

	// JujuMongodPath holds the default path to the juju-specific mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"

	upstartConfInstall          = (*upstart.Conf).Install
	upstartServiceStopAndRemove = (*upstart.Service).StopAndRemove
	upstartServiceStop          = (*upstart.Service).Stop
	upstartServiceStart         = (*upstart.Service).Start
)

// WithAddresses represents an entity that has a set of
// addresses. e.g. a state Machine object
type WithAddresses interface {
	Addresses() []instance.Address
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

	machinePeerAddr := SelectPeerAddress(addrs)
	return machinePeerAddr == masterAddr, nil
}

// SelectPeerAddress returns the address to use as the
// mongo replica set peer address by selecting it from the given addresses.
func SelectPeerAddress(addrs []instance.Address) string {
	return instance.SelectInternalAddress(addrs, false)
}

// SelectPeerHostPort returns the HostPort to use as the
// mongo replica set peer by selecting it from the given hostPorts.
func SelectPeerHostPort(hostPorts []instance.HostPort) string {
	return instance.SelectInternalHostPort(hostPorts, false)
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

// RemoveService removes the mongoDB upstart service from this machine.
func RemoveService(namespace string) error {
	svc := upstart.NewService(ServiceName(namespace))
	return upstartServiceStopAndRemove(svc)
}

// EnsureMongoServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
// before installing the new version.
//
// The namespace is a unique identifier to prevent multiple instances of mongo
// on this machine from colliding. This should be empty unless using
// the local provider.
func EnsureServer(dataDir string, namespace string, info params.StateServingInfo) error {
	logger.Infof("Ensuring mongo server is running; data directory %s; port %d", dataDir, info.StatePort)
	dbDir := filepath.Join(dataDir, "db")

	if err := os.MkdirAll(dbDir, 0700); err != nil {
		return fmt.Errorf("cannot create mongo database directory: %v", err)
	}

	certKey := info.Cert + "\n" + info.PrivateKey
	err := utils.AtomicWriteFile(sslKeyPath(dataDir), []byte(certKey), 0600)
	if err != nil {
		return fmt.Errorf("cannot write SSL key: %v", err)
	}

	err = utils.AtomicWriteFile(sharedSecretPath(dataDir), []byte(info.SharedSecret), 0600)
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

	if err := aptGetInstallMongod(); err != nil {
		return fmt.Errorf("cannot install mongod: %v", err)
	}

	upstartConf, mongoPath, err := upstartService(namespace, dataDir, dbDir, info.StatePort)
	if err != nil {
		return err
	}
	logVersion(mongoPath)

	if err := upstartServiceStop(&upstartConf.Service); err != nil {
		return fmt.Errorf("failed to stop mongo: %v", err)
	}
	if err := makeJournalDirs(dbDir); err != nil {
		return fmt.Errorf("error creating journal directories: %v", err)
	}
	return upstartConfInstall(upstartConf)
}

// ServiceName returns the name of the upstart service config for mongo using
// the given namespace.
func ServiceName(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s-%s", serviceName, namespace)
	}
	return serviceName
}

func makeJournalDirs(dataDir string) error {
	journalDir := path.Join(dataDir, "journal")

	if err := os.MkdirAll(journalDir, 0700); err != nil {
		logger.Errorf("failed to make mongo journal dir %s: %v", journalDir, err)
		return err
	}

	// manually create the prealloc files, since otherwise they get created as 100M files.
	zeroes := make([]byte, 64*1024) // should be enough for anyone
	for x := 0; x < 3; x++ {
		name := fmt.Sprintf("prealloc.%d", x)
		filename := filepath.Join(journalDir, name)
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
		// TODO(jam) 2014-04-12 https://launchpad.net/bugs/1306902
		// When we support upgrading Mongo into Replica mode, we should
		// start rewriting the upstart config
		if os.IsExist(err) {
			// already exists, don't overwrite
			continue
		}
		if err != nil {
			return fmt.Errorf("failed to open mongo prealloc file %q: %v", filename, err)
		}
		defer f.Close()
		for total := 0; total < 1024*1024; {
			n, err := f.Write(zeroes)
			if err != nil {
				return fmt.Errorf("failed to write to mongo prealloc file %q: %v", filename, err)
			}
			total += n
		}
	}
	return nil
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

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, "server.pem")
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

// upstartService returns the upstart config for the mongo state service.
// It also returns the path to the mongod executable that the upstart config
// will be using.
func upstartService(namespace, dataDir, dbDir string, port int) (*upstart.Conf, string, error) {
	svc := upstart.NewService(ServiceName(namespace))

	mongoPath, err := Path()
	if err != nil {
		return nil, "", err
	}

	mongoCmd := mongoPath + " --auth" +
		" --dbpath=" + utils.ShQuote(dbDir) +
		" --sslOnNormalPorts" +
		" --sslPEMKeyFile " + utils.ShQuote(sslKeyPath(dataDir)) +
		" --sslPEMKeyPassword ignored" +
		" --bind_ip 0.0.0.0" +
		" --port " + fmt.Sprint(port) +
		" --noprealloc" +
		" --syslog" +
		" --smallfiles" +
		" --journal" +
		" --keyFile " + utils.ShQuote(sharedSecretPath(dataDir)) +
		" --replSet " + ReplicaSetName
	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxFiles, maxFiles),
			"nproc":  fmt.Sprintf("%d %d", maxProcs, maxProcs),
		},
		Cmd: mongoCmd,
	}
	return conf, mongoPath, nil
}

func aptGetInstallMongod() error {
	// Only Quantal requires the PPA.
	if version.Current.Series == "quantal" {
		if err := addAptRepository("ppa:juju/stable"); err != nil {
			return err
		}
	}
	pkg := packageForSeries(version.Current.Series)
	cmds := apt.GetPreparePackages([]string{pkg}, version.Current.Series)
	logger.Infof("installing %s", pkg)
	for _, cmd := range cmds {
		if err := apt.GetInstall(cmd...); err != nil {
			return err
		}
	}
	return nil
}

func addAptRepository(name string) error {
	// add-apt-repository requires python-software-properties
	cmds := apt.GetPreparePackages(
		[]string{"python-software-properties"},
		version.Current.Series,
	)
	logger.Infof("installing python-software-properties")
	for _, cmd := range cmds {
		if err := apt.GetInstall(cmd...); err != nil {
			return err
		}
	}

	logger.Infof("adding apt repository %q", name)
	cmd := exec.Command("add-apt-repository", "-y", name)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("cannot add apt repository: %v (output %s)", err, bytes.TrimSpace(out))
	}
	return nil
}

// packageForSeries returns the name of the mongo package for the series
// of the machine that it is going to be running on.
func packageForSeries(series string) string {
	switch series {
	case "precise", "quantal", "raring", "saucy":
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
