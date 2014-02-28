package mongo

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/upstart"
	"launchpad.net/juju-core/utils"
)

const (
	maxMongoFiles         = 65000
	jujuMongodDefaultPath = "/usr/lib/juju/bin/mongod"
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	oldMongoServiceName = "juju-db"

	// this value is what we use in the code, it's a variable so we can mock it
	// out
	jujuMongodPath = jujuMongodDefaultPath
)

// MockPackage mocks out specific parts of this package for testing purposes.
// This function should not be called from production code.
func MockPackage() func() {
	jujuMongodPath = "/somewhere/that/doesnt/exist"
	return func() { jujuMongodPath = jujuMongodDefaultPath }
}

// MongoPath returns the executable path to be used to run mongod on this
// machine. If the juju-bundled version of mongo exists, it will return that
// path, otherwise it will return the command to run mongod from the path.
func MongodPath() string {
	if _, err := os.Stat(jujuMongodPath); err == nil {
		return jujuMongodPath
	}

	// just use whatever is in the path
	return "mongod"
}

// ensureMongoServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
// before installing the new version.
func ensureMongoServer(dir string, port int) error {
	name := makeServiceName(mongoScriptVersion)
	service := MongoUpstartService(name, dir, port)
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
		logger.Errorf("Failed to install mongo service %q: %v", service.Name, err)
		return err
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
	zeroes := make([]byte, 1024*1024)
	for x := 0; x < 3; x++ {
		name := fmt.Sprintf("prealloc.%d", x)
		filename := filepath.Join(journalDir, name)
		if err := ioutil.WriteFile(filename, zeroes, 700); err != nil {
			logger.Errorf("failed to make write mongo prealloc file: %v", journalDir, err)
			return err
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

// MongoUpstartService returns the upstart config for the mongo state service
// and the version number of that config.
//
// This method assumes there is a server.pem keyfile in dataDir.
func MongoUpstartService(name, dataDir string, port int) *upstart.Conf {

	keyFile := path.Join(dataDir, "server.pem")
	svc := upstart.NewService(name)

	dbDir := path.Join(dataDir, "db")

	conf := &upstart.Conf{
		Service: *svc,
		Desc:    "juju state database",
		Limit: map[string]string{
			"nofile": fmt.Sprintf("%d %d", maxMongoFiles, maxMongoFiles),
			"nproc":  fmt.Sprintf("%d %d", upstart.MaxAgentFiles, upstart.MaxAgentFiles),
		},
		Cmd: MongodPath() +
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
	return conf
}
