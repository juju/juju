// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localstorage

import (
	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/agent"
	"launchpad.net/juju-core/environs/filestorage"
	"launchpad.net/juju-core/environs/httpstorage"
	"launchpad.net/juju-core/worker"
)

var logger = loggo.GetLogger("juju.worker.localstorage")

type storageWorker struct {
	config agent.Config
	tomb   tomb.Tomb
}

func NewWorker(config agent.Config) worker.Worker {
	w := &storageWorker{config: config}
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
	storageDir := s.config.Value(agent.StorageDir)
	storageAddr := s.config.Value(agent.StorageAddr)
	logger.Infof("serving %s on %s", storageDir, storageAddr)

	storage, err := filestorage.NewFileStorageWriter(storageDir, filestorage.UseDefaultTmpDir)
	if err != nil {
		logger.Errorf("error with local storage: %v", err)
		return err
	}
	storageListener, err := httpstorage.Serve(storageAddr, storage)
	if err != nil {
		logger.Errorf("error with local storage: %v", err)
		return err
	}
	defer storageListener.Close()

	sharedStorageDir := s.config.Value(agent.SharedStorageDir)
	sharedStorageAddr := s.config.Value(agent.SharedStorageAddr)
	if sharedStorageAddr != "" && sharedStorageDir != "" {
		logger.Infof("serving %s on %s", sharedStorageDir, sharedStorageAddr)
		sharedStorage, err := filestorage.NewFileStorageWriter(sharedStorageDir, filestorage.UseDefaultTmpDir)
		if err != nil {
			logger.Errorf("error with local storage: %v", err)
			return err
		}
		sharedStorageListener, err := httpstorage.Serve(sharedStorageAddr, sharedStorage)
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
