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

	// SharedSecretFile is the name of the Mongo shared secret file
	// located within the Juju data directory.
	SharedSecretFile = "shared-secret"
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	oldMongoServiceName = "juju-db"

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
	// If the replica set has not been initiated, the first
	// address of DialInfo.Addrs is used as the address
	// of the first replica set member.
	DialInfo *mgo.DialInfo

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
	if err == mgo.ErrNotFound {
		err := replicaset.Initiate(session, p.DialInfo.Addrs[0], replicaSetName)
		if err != nil {
			return fmt.Errorf("cannot initiate replica set: %v", err)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("cannot get replica set configuration: %v", err)
	}
	return nil
}

// EnsureMongoServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
// before installing the new version.
func EnsureMongoServer(dataDir string, port int) error {
	// TODO(natefinch): write out keyfile and shared secret

	logger.Debugf("Ensuring mongo server is running; dataDir %s; port %d", dataDir, port)
	dbDir := filepath.Join(dataDir, "db")
	name := makeServiceName(mongoScriptVersion)

	if err := removeOldMongoServices(mongoScriptVersion); err != nil {
		return err
	}
	service, err := mongoUpstartService(name, dataDir, dbDir, port)
	if err != nil {
		return err
	}
	if !service.Installed() {
		if err := makeJournalDirs(dbDir); err != nil {
			return fmt.Errorf("Error creating journal directories: %v", err)
		}
		logger.Debugf("mongod upstart command: %s", service.Cmd)
		err = service.Install()
		if err != nil {
			return fmt.Errorf("failed to install mongo service %q: %v", service.Name, err)
		}
	}
	if !service.Running() {
		if err := service.Start(); err != nil {
			return fmt.Errorf("failed to start %q service: %v", name, err)
		}
		logger.Infof("Mongod service %q started.", name)
	}
	return nil
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
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0700)
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

// removeOldMongoServices looks for any old juju mongo upstart scripts and
// removes them.
func removeOldMongoServices(curVersion int) error {
	old := upstart.NewService(oldMongoServiceName)
	if err := old.StopAndRemove(); err != nil {
		logger.Errorf("failed to remove old mongo upstart service %q: %v", old.Name, err)
		return err
	}

	// the new formatting for the script name started at version 2
	for x := 2; x < curVersion; x++ {
		old := upstart.NewService(makeServiceName(x))
		if err := old.StopAndRemove(); err != nil {
			logger.Errorf("failed to remove old mongo upstart service %q: %v", old.Name, err)
			return err
		}
	}
	return nil
}

// ServiceName returns a string for the current juju db version
func ServiceName() string {
	return makeServiceName(mongoScriptVersion)
}

func makeServiceName(version int) string {
	return fmt.Sprintf("juju-db-v%d", version)
}

// RemoveService will stop and remove Juju's mongo upstart service.
func RemoveService() error {
	svc := upstart.NewService(ServiceName())
	return svc.StopAndRemove()
}

// mongoScriptVersion keeps track of changes to the mongo upstart script.
// Update this version when you update the script that gets installed from
// MongoUpstartService.
const mongoScriptVersion = 2

// mongoUpstartService returns the upstart config for the mongo state service.
//
// This method assumes there exist "server.pem" and "shared_secret" keyfiles in dataDir.
func mongoUpstartService(name, dataDir, dbDir string, port int) (*upstart.Conf, error) {
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
