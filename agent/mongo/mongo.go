package mongo

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/upstart"
)

const (
	mongoSvcFmt         = "juju-db-v%d"
	oldMongoServiceName = "juju-db"
)

var (
	logger = loggo.GetLogger("juju.agent.mongo")

	mongoServiceName = fmt.Sprintf(mongoSvcFmt, upstart.MongoScriptVersion)
)

// ensureMongoServer ensures that the correct mongo upstart script is installed
// and running.
//
// This method will remove old versions of the mongo upstart script as necessary
// before installing the new version.
func ensureMongoServer(dir string, port int) error {
	service := upstart.MongoUpstartService(mongoServiceName, dir, port)
	if service.Installed() {
		return nil
	}

	if err := removeOldMongoServices(); err != nil {
		return err
	}

	journalDir := filepath.Join(mongoDir, "journal")

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
	for x := 2; x < upstart.MongoScriptVersion; x++ {
		old := upstart.NewService(fmt.Sprintf(mongoSvcFmt, x))
		if err := old.StopAndRemove(); err != nil {
			logger.Errorf("Failed to remove old mongo upstart service %q: %v", old.Name, err)
			return err
		}
	}
	return nil
}
