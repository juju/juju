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
	"strconv"

	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/juju/utils/apt"
	"gopkg.in/mgo.v2"

	"github.com/juju/juju/network"
	"github.com/juju/juju/replicaset"
	"github.com/juju/juju/service/common"
	"github.com/juju/juju/service/upstart"
	"github.com/juju/juju/version"
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
	logger          = loggo.GetLogger("juju.mongo")
	mongoConfigPath = "/etc/default/mongodb"

	// JujuMongodPath holds the default path to the juju-specific mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"

	upstartConfInstall          = (*upstart.Service).Install
	upstartServiceExists        = (*upstart.Service).Exists
	upstartServiceRunning       = (*upstart.Service).Running
	upstartServiceStopAndRemove = (*upstart.Service).StopAndRemove
	upstartServiceStop          = (*upstart.Service).Stop
	upstartServiceStart         = (*upstart.Service).Start

	// This is NUMACTL package name for apt-get
	numaCtlPkg = "numactl"
	// This is the name of the variable to use in ExtraScript
	// fragment to substitu into upstart template.
	multinodeVarName = "MULTI_NODE"
	// This value will be used to wrap desired mongo cmd in numactl if wanted/needed
	numaCtlWrap = "$%v"
	// Extra shell script fragment for upstart template.
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

	machinePeerAddr := SelectPeerAddress(addrs)
	return machinePeerAddr == masterAddr, nil
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

// RemoveService removes the mongoDB upstart service from this machine.
func RemoveService(namespace string) error {
	svc := upstart.NewService(ServiceName(namespace), common.Conf{})
	return upstartServiceStopAndRemove(svc)
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

// EnsureServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
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

	if err := aptGetInstallMongod(args.SetNumaControlPolicy); err != nil {
		return fmt.Errorf("cannot install mongod: %v", err)
	}
	mongoPath, err := Path()
	if err != nil {
		return err
	}
	logVersion(mongoPath)

	svc, err := upstartService(args.Namespace, args.DataDir, dbDir, mongoPath, args.StatePort, oplogSizeMB, args.SetNumaControlPolicy)
	if err != nil {
		return err
	}
	if upstartServiceExists(svc) {
		logger.Debugf("mongo exists as expected")
		if !upstartServiceRunning(svc) {
			return upstartServiceStart(svc)
		}
		return nil
	}

	certKey := args.Cert + "\n" + args.PrivateKey
	err = utils.AtomicWriteFile(sslKeyPath(args.DataDir), []byte(certKey), 0600)
	if err != nil {
		return fmt.Errorf("cannot write SSL key: %v", err)
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

	if err := upstartServiceStop(svc); err != nil {
		return fmt.Errorf("failed to stop mongo: %v", err)
	}
	if err := makeJournalDirs(dbDir); err != nil {
		return fmt.Errorf("error creating journal directories: %v", err)
	}
	if err := preallocOplog(dbDir, oplogSizeMB); err != nil {
		return fmt.Errorf("error creating oplog files: %v", err)
	}
	return upstartConfInstall(svc)
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

func sslKeyPath(dataDir string) string {
	return filepath.Join(dataDir, "server.pem")
}

func sharedSecretPath(dataDir string) string {
	return filepath.Join(dataDir, SharedSecretFile)
}

// upstartService returns the upstart config for the mongo state service.
// It also returns the path to the mongod executable that the upstart config
// will be using.
func upstartService(namespace, dataDir, dbDir, mongoPath string, port, oplogSizeMB int, wantNumaCtl bool) (*upstart.Service, error) {
	mongoCmd := mongoPath + " --auth" +
		" --dbpath=" + utils.ShQuote(dbDir) +
		" --sslOnNormalPorts" +
		" --sslPEMKeyFile " + utils.ShQuote(sslKeyPath(dataDir)) +
		" --sslPEMKeyPassword ignored" +
		" --port " + fmt.Sprint(port) +
		" --noprealloc" +
		" --syslog" +
		" --smallfiles" +
		" --journal" +
		" --keyFile " + utils.ShQuote(sharedSecretPath(dataDir)) +
		" --replSet " + ReplicaSetName +
		" --ipv6 " +
		" --oplogSize " + strconv.Itoa(oplogSizeMB)
	extraScript := ""
	if wantNumaCtl {
		extraScript = fmt.Sprintf(detectMultiNodeScript, multinodeVarName, multinodeVarName)
		mongoCmd = fmt.Sprintf(numaCtlWrap, multinodeVarName) + mongoCmd
	}
	conf := common.Conf{
		Desc: "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxFiles, maxFiles),
			"nproc":  fmt.Sprintf("%d %d", maxProcs, maxProcs),
		},
		ExtraScript: extraScript,
		Cmd:         mongoCmd,
	}
	svc := upstart.NewService(ServiceName(namespace), conf)
	return svc, nil
}

func aptGetInstallMongod(numaCtl bool) error {
	// Only Quantal requires the PPA.
	if version.Current.Series == "quantal" {
		if err := addAptRepository("ppa:juju/stable"); err != nil {
			return err
		}
	}
	mongoPkg := packageForSeries(version.Current.Series)

	aptPkgs := []string{mongoPkg}
	if numaCtl {
		aptPkgs = []string{mongoPkg, numaCtlPkg}
		logger.Infof("installing %s and %s", mongoPkg, numaCtlPkg)
	} else {
		logger.Infof("installing %s", mongoPkg)
	}
	cmds := apt.GetPreparePackages(aptPkgs, version.Current.Series)
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
