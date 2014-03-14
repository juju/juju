package mongo

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

const (
	maxFiles = 65000
	maxProcs = 20000
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	oldMongoServiceName = "juju-db"

	// JujuMongodPath holds the default path to the juju-specific mongod.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"
)

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

// EnsureMongoServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
// before installing the new version.
func EnsureMongoServer(dir string, port int) error {
	name := makeServiceName(mongoScriptVersion)
	service, err := MongoUpstartService(name, dir, port)
	if err != nil {
		return err
	}
	if service.Installed() {
		return nil
	}

	if err := removeOldMongoServices(mongoScriptVersion); err != nil {
		return err
	}

	if err := makeJournalDirs(dir); err != nil {
		return err
	}

	if err := service.Install(); err != nil {
		return fmt.Errorf("failed to install mongo service %q: %v", service.Name, err)
	}
	return service.Start()
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
		logger.Errorf("Failed to remove old mongo upstart service %q: %v", old.Name, err)
		return err
	}

	// the new formatting for the script name started at version 2
	for x := 2; x < curVersion; x++ {
		old := upstart.NewService(makeServiceName(x))
		if err := old.StopAndRemove(); err != nil {
			logger.Errorf("Failed to remove old mongo upstart service %q: %v", old.Name, err)
			return err
		}
	}
	return nil
}

func makeServiceName(version int) string {
	return fmt.Sprintf("juju-db-v%d", version)
}

// mongoScriptVersion keeps track of changes to the mongo upstart script.
// Update this version when you update the script that gets installed from
// MongoUpstartService.
const mongoScriptVersion = 2

// MongoUpstartService returns the upstart config for the mongo state service.
//
// This method assumes there is a server.pem keyfile in dataDir.
func MongoUpstartService(name, dataDir string, port int) (*upstart.Conf, error) {

	keyFile := path.Join(dataDir, "server.pem")
	svc := upstart.NewService(name)

	dbDir := path.Join(dataDir, "db")

	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxFiles, maxFiles),
			"nproc":  fmt.Sprintf("%d %d", maxProcs, maxProcs),
		},
		Cmd: "/usr/bin/mongod" +
			" --auth" +
			" --dbpath=" + dbDir +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile " + utils.ShQuote(keyFile) +
			" --sslPEMKeyPassword ignored" +
			" --bind_ip 0.0.0.0" +
			" --port " + fmt.Sprint(port) +
			" --noprealloc" +
			" --syslog" +
			" --smallfiles",
		// TODO(Nate): uncomment when we commit HA stuff
		// +
		//	" --replSet juju",
	}
	return conf, nil
}
