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
	mongoSvcFmt         = "juju-db-v%d"
	oldMongoServiceName = "juju-db"

	maxMongoFiles = 65000

	// mongoScriptVersion keeps track of changes to the mongo upstart script.
	// Update this version when you update the script that gets installed from
	// MongoUpstartService.
	mongoScriptVersion = 2
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	mongoServiceName = fmt.Sprintf(mongoSvcFmt, mongoScriptVersion)

	// JujuMongodPath is the path of the mongod that is bundled specifically for
	// juju. This value is public and non-const only for testing purposes,
	// please do not change.
	JujuMongodPath = "/usr/lib/juju/bin/mongod"
)

// MongoPath returns the executable path to be used to run mongod on this
// machine. If the juju-bundled version of mongo exists, it will return that
// path, otherwise it will return the command to run mongod from the path.
func MongodPath() string {
	if _, err := os.Stat(JujuMongodPath); err == nil {
		return JujuMongodPath
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
	service := MongoUpstartService(mongoServiceName, dir, port)
	if service.Installed() {
		return nil
	}

	if err := removeOldMongoServices(); err != nil {
		return err
	}

	journalDir := filepath.Join(dir, "journal")

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

	if err := service.Install(); err != nil {
		logger.Errorf("Failed to install mongo service %q: %v", service.Name, err)
		return err
	}
	return service.Start()
}

// removeOldMongoServices looks for any old juju mongo upstart scripts and
// removes them.
func removeOldMongoServices() error {
	old := upstart.NewService(oldMongoServiceName)
	if err := old.StopAndRemove(); err != nil {
		logger.Errorf("Failed to remove old mongo upstart service %q: %v", old.Name, err)
		return err
	}

	// the new formatting for the script name started at version 2
	for x := 2; x < mongoScriptVersion; x++ {
		old := upstart.NewService(fmt.Sprintf(mongoSvcFmt, x))
		if err := old.StopAndRemove(); err != nil {
			logger.Errorf("Failed to remove old mongo upstart service %q: %v", old.Name, err)
			return err
		}
	}
	return nil
}

// MongoUpstartService returns the upstart config for the mongo state service.
//
// This method assumes there is a server.pem keyfile in dataDir.
func MongoUpstartService(name, dataDir string, port int) *upstart.Conf {
	keyFile := path.Join(dataDir, "server.pem")
	svc := upstart.NewService(name)

	dbDir := path.Join(dataDir, "db")

	return &upstart.Conf{
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
}
