// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	"gopkg.in/tomb.v2"
)

var logger = loggo.GetLogger("juju.worker.hostkeyreporter")

// Facade exposes controller functionality to a Worker.
type Facade interface {
	ReportKeys(machineId string, publicKeys []string) error
}

// Config defines the parameters of the hostkeyreporter worker.
type Config struct {
	Facade    Facade
	MachineId string
	RootDir   string
}

// Validate returns an error if Config cannot drive a hostkeyreporter.
func (config Config) Validate() error {
	if config.Facade == nil {
		return errors.NotValidf("nil Facade")
	}
	if config.MachineId == "" {
		return errors.NotValidf("empty MachineId")
	}
	return nil
}

// New returns a Worker backed by config, or an error.
func New(config Config) (worker.Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, errors.Trace(err)
	}
	w := &hostkeyreporter{config: config}
	w.tomb.Go(w.run)
	return w, nil
}

// Worker waits for a model migration to be active, then locks down the
// configured fortress and implements the migration.
type hostkeyreporter struct {
	tomb   tomb.Tomb
	config Config
}

// Kill implements worker.Worker.
func (w *hostkeyreporter) Kill() {
	w.tomb.Kill(nil)
}

// Wait implements worker.Worker.
func (w *hostkeyreporter) Wait() error {
	return w.tomb.Wait()
}

func (w *hostkeyreporter) run() error {
	keys, err := w.readSSHKeys()
	if err != nil {
		return errors.Trace(err)
	}
	if len(keys) < 1 {
		return errors.New("no SSH host keys found")
	}
	err = w.config.Facade.ReportKeys(w.config.MachineId, keys)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf("%d SSH host keys reported for machine %s", len(keys), w.config.MachineId)
	return dependency.ErrUninstall
}

func (w *hostkeyreporter) readSSHKeys() ([]string, error) {
	sshDir := w.sshDir()
	_, err := os.Stat(sshDir)
	if os.IsNotExist(err) {
		logger.Errorf("%s doesn't exist - giving up", sshDir)
		return nil, dependency.ErrUninstall
	}
	if err != nil {
		return nil, errors.Trace(err)
	}

	filenames, err := filepath.Glob(sshDir + "/ssh_host_*_key.pub")
	if err != nil {
		return nil, errors.Trace(err)
	}
	keys := make([]string, 0, len(filenames))
	for _, filename := range filenames {
		key, err := ioutil.ReadFile(filename)
		if err != nil {
			logger.Debugf("unable to read SSH host key (skipping): %v", err)
			continue
		}
		keys = append(keys, string(key))
	}
	return keys, nil
}

func (w *hostkeyreporter) sshDir() string {
	return filepath.Join(w.config.RootDir, "etc", "ssh")
}
