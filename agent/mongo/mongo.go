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

	"github.com/juju/loggo"
	"labix.org/v2/mgo"

	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/replicaset"
	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

const (
	maxFiles = 65000
	maxProcs = 20000

	replicaSetName = "juju"

	serviceName = "juju-db"

	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory.
	SharedSecretFile = "shared-secret"
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	// JujuMongodPath holds the default path to the juju-specific mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"
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

// MongoPath returns the executable path to be used to run mongod on this
// machine. If the juju-bundled version of mongo exists, it will return that
// path, otherwise it will return the command to run mongod from the path.
func MongodPath() (string, error) {
	if _, err := os.Stat(JujuMongodPath); err == nil {
		return JujuMongodPath, nil
	}

	path, err := exec.LookPath("mongod")
	if err != nil {
		return "", err
	}
	return path, nil
}

// InitiateMongoParams holds parameters for the MaybeInitiateMongo call.
type InitiateMongoParams struct {
	// DialInfo specifies how to connect to the mongo server.
	DialInfo *mgo.DialInfo

	// MemberHostPort provides the address to use for
	// the first replica set member.
	MemberHostPort string

	// User holds the user to log as in to the mongo server.
	// If it is empty, no login will take place.
	User     string
	Password string
}

// MaybeInitiateMongoServer checks for an existing mongo configuration.
// If no existing configuration is found one is created using Initiate.
func MaybeInitiateMongoServer(p InitiateMongoParams) error {
	logger.Debugf("Initiating mongo replicaset; params: %#v", p)

	if len(p.DialInfo.Addrs) > 1 {
		logger.Infof("more than one member; replica set must be already initiated")
		return nil
	}
	p.DialInfo.Direct = true
	session, err := mgo.DialWithInfo(p.DialInfo)
	if err != nil {
		return fmt.Errorf("can't dial mongo to initiate replicaset: %v", err)
	}
	defer session.Close()

	// TODO(rog) remove this code when we no longer need to upgrade
	// from pre-HA-capable environments.
	if p.User != "" {
		err := session.DB("admin").Login(p.User, p.Password)
		if err != nil {
			logger.Errorf("cannot login to admin db as %q, password %q, falling back: %v", p.User, p.Password, err)
		}
	}
	_, err = replicaset.CurrentConfig(session)
	if err == nil {
		// already initiated, nothing to do
		return nil
	}
	if err != mgo.ErrNotFound {
		// oops, some random error, bail
		return fmt.Errorf("cannot get replica set configuration: %v", err)
	}

	// err is ErrNotFound, which just means we need to initiate

	err = replicaset.Initiate(session, p.MemberHostPort, replicaSetName)
	if err != nil {
		return fmt.Errorf("cannot initiate replica set: %v", err)
	}
	return nil
}

// RemoveService removes the mongoDB upstart service from this machine.
func RemoveService(namespace string) error {
	return upstart.NewService(ServiceName(namespace)).StopAndRemove()
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
func EnsureMongoServer(dataDir string, port int, namespace string) error {
	// NOTE: ensure that the right package is installed?
	logger.Infof("Ensuring mongo server is running; dataDir %s; port %d", dataDir, port)
	dbDir := filepath.Join(dataDir, "db")

	service, err := mongoUpstartService(namespace, dataDir, dbDir, port)
	if err != nil {
		return err
	}

	// TODO(natefinch) 2014-04-12 https://launchpad.net/bugs/1306902
	// remove this once we support upgrading to HA
	if service.Installed() {
		return nil
	}

	if err := makeJournalDirs(dbDir); err != nil {
		return fmt.Errorf("Error creating journal directories: %v", err)
	}
	return service.Install()
}

// ServiceName returns the name of the upstart service config for mongo using
// the given namespace.
func ServiceName(namespace string) string {
	if namespace != "" {
		return fmt.Sprintf("%s-%s", serviceName, namespace)
	}
	return serviceName
}

func makeJournalDirs(dir string) error {
	journalDir := path.Join(dir, "journal")

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

// mongoUpstartService returns the upstart config for the mongo state service.
//
// This method assumes there exist "server.pem" and "shared_secret" keyfiles in dataDir.
func mongoUpstartService(namespace, dataDir, dbDir string, port int) (*upstart.Conf, error) {
	// NOTE: ensure that the right package is installed?
	name := ServiceName(namespace)
	sslKeyFile := path.Join(dataDir, "server.pem")

	// TODO (natefinch) uncomment when we have the keyfile
	// keyFile := path.Join(dataDir, SharedSecretFile)
	svc := upstart.NewService(name)

	mongopath, err := MongodPath()
	if err != nil {
		return nil, err
	}

	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxFiles, maxFiles),
			"nproc":  fmt.Sprintf("%d %d", maxProcs, maxProcs),
		},
		Cmd: mongopath + " --auth" +
			" --dbpath=" + dbDir +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile " + utils.ShQuote(sslKeyFile) +
			" --sslPEMKeyPassword ignored" +
			" --bind_ip 0.0.0.0" +
			" --port " + fmt.Sprint(port) +
			" --noprealloc" +
			" --syslog" +
			" --smallfiles" +
			" --replSet " + replicaSetName,
		// TODO(natefinch) uncomment when we have the keyfile
		//" --keyFile " + utils.ShQuote(keyFile),
	}
	return conf, nil
}

// mongoNoauthCommand returns an os/exec.Cmd that may be executed to
// run mongod without security.
func mongoNoauthCommand(dataDir string, port int) (*exec.Cmd, error) {
	sslKeyFile := path.Join(dataDir, "server.pem")
	dbDir := filepath.Join(dataDir, "db")
	mongopath, err := MongodPath()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(mongopath,
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
	)
	return cmd, nil
}
