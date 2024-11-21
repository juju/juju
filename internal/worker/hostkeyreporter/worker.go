// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package hostkeyreporter

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	"gopkg.in/tomb.v2"

	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/internal/wrench"
)

var logger = internallogger.GetLogger("juju.worker.hostkeyreporter")

// Facade exposes controller functionality to a Worker.
type Facade interface {
	ReportKeys(ctx context.Context, machineId string, publicKeys []string) error
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
	if wrench.IsActive("hostkeyreporter", "delay") {
		time.Sleep(time.Minute)
	}
	keys, err := w.readSSHKeys()
	if err != nil {
		return errors.Trace(err)
	}
	if len(keys) < 1 {
		return errors.New("no SSH host keys found")
	}
	err = w.config.Facade.ReportKeys(context.TODO(), w.config.MachineId, keys)
	if err != nil {
		return errors.Trace(err)
	}
	logger.Debugf(context.TODO(), "%d SSH host keys reported for machine %s", len(keys), w.config.MachineId)
	return dependency.ErrUninstall
}

func (w *hostkeyreporter) readSSHKeys() ([]string, error) {
	sshDir := w.sshDir()
	_, err := os.Stat(sshDir)
	if os.IsNotExist(err) {
		logger.Errorf(context.TODO(), "%s doesn't exist - giving up", sshDir)
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
		key, err := os.ReadFile(filename)
		if err != nil {
			logger.Debugf(context.TODO(), "unable to read SSH host key (skipping): %v", err)
			continue
		}
		keys = append(keys, string(key))
	}
	return keys, nil
}

func (w *hostkeyreporter) sshDir() string {
	return filepath.Join(w.config.RootDir, "etc", "ssh")
}
