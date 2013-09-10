// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage

import (
	"os"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs/localstorage"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.local.storage")

type storageWorker struct {
	tomb tomb.Tomb
}

func NewWorker() worker.Worker {
	w := &storageWorker{}
	go func() {
		defer w.tomb.Done()
		w.tomb.Kill(w.waitForDeath())
	}()
	return w
}

// Kill implements worker.Worker.Kill.
func (s *storageWorker) Kill() {
	s.tomb.Kill(nil)
}

// Wait implements worker.Worker.Wait.
func (s *storageWorker) Wait() error {
	return s.tomb.Wait()
}

func (s *storageWorker) waitForDeath() error {
	storageDir := os.Getenv(osenv.JujuStorageDir)
	storageAddr := os.Getenv(osenv.JujuStorageAddr)
	logger.Infof("serving %s on %s", storageDir, storageAddr)

	storageListener, err := localstorage.Serve(storageAddr, storageDir)
	if err != nil {
		logger.Errorf("error with local storage: %v", err)
		return err
	}
	defer storageListener.Close()

	sharedStorageDir := os.Getenv(osenv.JujuSharedStorageDir)
	sharedStorageAddr := os.Getenv(osenv.JujuSharedStorageAddr)
	if sharedStorageAddr != "" && sharedStorageDir != "" {
		logger.Infof("serving %s on %s", sharedStorageDir, sharedStorageAddr)

		sharedStorageListener, err := localstorage.Serve(sharedStorageAddr, sharedStorageDir)
		if err != nil {
			logger.Errorf("error with local storage: %v", err)
			return err
		}
		defer sharedStorageListener.Close()
	} else {
		logger.Infof("no shared storage: dir=%q addr=%q", sharedStorageDir, sharedStorageAddr)
	}

	logger.Infof("storage routines started, awaiting death")

	<-s.tomb.Dying()

	logger.Infof("dying, closing storage listeners")
	return tomb.ErrDying
}
