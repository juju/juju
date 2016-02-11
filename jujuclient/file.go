// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jujuclient provides functionality to support
// connections to Juju such as controllers cache, accounts cache, etc.

package jujuclient

import (
	"fmt"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/retry"
	"github.com/juju/utils/clock"
	// TODO(axw) replace with flock on file in $XDG_RUNTIME_DIR
	"github.com/juju/utils/fslock"

	"github.com/juju/juju/juju/osenv"
)

var logger = loggo.GetLogger("juju.jujuclient")

// A second should be enough to write or read any files. But
// some disks are slow when under load, so lets give the disk a
// reasonable time to get the lock.
var lockTimeout = 5 * time.Second

// NewFileClientStore returns a new filesystem-based client store
// that manages files in $XDG_DATA_HOME/juju.
func NewFileClientStore() ClientStore {
	return &store{}
}

type store struct{}

func (s *store) lock(operation string) (*fslock.Lock, error) {
	lockName := "controllers.lock"
	lock, err := fslock.NewLock(osenv.JujuXDGDataHome(), lockName, fslock.Defaults())
	if err != nil {
		return nil, errors.Trace(err)
	}
	message := fmt.Sprintf("pid: %d, operation: %s", os.Getpid(), operation)
	err = lock.LockWithTimeout(lockTimeout, message)
	if err == nil {
		return lock, nil
	}
	if errors.Cause(err) != fslock.ErrTimeout {
		return nil, errors.Trace(err)
	}

	logger.Warningf("breaking jujuclient lock : %s", lockName)
	logger.Warningf("  lock holder message: %s", lock.Message())

	// If we are unable to acquire the lock within the lockTimeout,
	// consider it broken for some reason, and break it.
	err = lock.BreakLock()
	if err != nil {
		return nil, errors.Annotatef(err, "unable to break the jujuclient lock %v", lockName)
	}

	err = lock.LockWithTimeout(lockTimeout, message)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return lock, nil
}

// It appears that sometimes the lock is not cleared when we expect it to be.
// Capture and log any errors from the Unlock method and retry a few times.
func (s *store) unlock(lock *fslock.Lock) {
	err := retry.Call(retry.CallArgs{
		Func: lock.Unlock,
		NotifyFunc: func(err error, attempt int) {
			logger.Debugf("failed to unlock jujuclient lock: %s", err)
		},
		Attempts: 10,
		Delay:    50 * time.Millisecond,
		Clock:    clock.WallClock,
	})
	if err != nil {
		logger.Errorf("unable to unlock jujuclient lock: %s", err)
	}
}

// AllControllers implements ControllersGetter.AllControllers.
func (s *store) AllControllers() (map[string]ControllerDetails, error) {
	lock, err := s.lock("read-all-controllers")
	if err != nil {
		return nil, errors.Annotate(err, "cannot read all controllers")
	}
	defer s.unlock(lock)
	return ReadControllersFile(JujuControllersPath())
}

// ControllerByName implements ControllersGetter.ControllerByName.
func (s *store) ControllerByName(name string) (*ControllerDetails, error) {
	if err := ValidateControllerName(name); err != nil {
		return nil, errors.Trace(err)
	}

	lock, err := s.lock("read-controller-by-name")
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read controller %v", name)
	}
	defer s.unlock(lock)

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result, ok := controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// UpdateController implements ControllersUpdater.UpdateController.
func (s *store) UpdateController(name string, details ControllerDetails) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateControllerDetails(details); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("update-controller")
	if err != nil {
		return errors.Annotatef(err, "cannot update controller %v", name)
	}
	defer s.unlock(lock)

	all, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	if len(all) == 0 {
		all = make(map[string]ControllerDetails)
	}

	all[name] = details
	return WriteControllersFile(all)
}

// RemoveController implements ControllersRemover.RemoveController
func (s *store) RemoveController(name string) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("remove-controller")
	if err != nil {
		return errors.Annotatef(err, "cannot remove controller %v", name)
	}
	defer s.unlock(lock)

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	// Remove models for the controller.
	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := controllerModels[name]; ok {
		delete(controllerModels, name)
		if err := WriteModelsFile(controllerModels); err != nil {
			return errors.Trace(err)
		}
	}

	// Remove the controller.
	delete(controllers, name)
	return WriteControllersFile(controllers)
}

// UpdateModel implements ModelUpdater.
func (s *store) UpdateModel(controllerName, modelName string, details ModelDetails) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateModelDetails(details); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("update-model")
	if err != nil {
		return errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if controllerModels == nil {
		controllerModels = make(map[string]*ControllerModels)
	}
	models, ok := controllerModels[controllerName]
	if !ok {
		models = &ControllerModels{
			Models: make(map[string]ModelDetails),
		}
		controllerModels[controllerName] = models
	}
	if oldDetails, ok := models.Models[modelName]; ok && details == oldDetails {
		return nil
	}

	models.Models[modelName] = details
	return errors.Trace(WriteModelsFile(controllerModels))
}

// CurrentModel implements ModelUpdater.
func (s *store) SetCurrentModel(controllerName, modelName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("set-current-model")
	if err != nil {
		return errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	models, ok := controllerModels[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if models.CurrentModel == modelName {
		return nil
	}
	if _, ok := models.Models[modelName]; !ok {
		return errors.NotFoundf("model %s:%s", controllerName, modelName)
	}

	models.CurrentModel = modelName
	return errors.Trace(WriteModelsFile(controllerModels))
}

// AllModels implements ModelGetter.
func (s *store) AllModels(controllerName string) (map[string]ModelDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}

	lock, err := s.lock("read-all-models")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	models, ok := controllerModels[controllerName]
	if !ok {
		return nil, errors.NotFoundf("models for controller %s", controllerName)
	}
	return models.Models, nil
}

// CurrentModel implements ModelGetter.
func (s *store) CurrentModel(controllerName string) (string, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return "", errors.Trace(err)
	}

	lock, err := s.lock("read-current-model")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return "", errors.Trace(err)
	}
	models, ok := controllerModels[controllerName]
	if !ok || models.CurrentModel == "" {
		return "", errors.NotFoundf("current model for controller %s", controllerName)
	}
	return models.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (s *store) ModelByName(controllerName, modelName string) (*ModelDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ValidateModelName(modelName); err != nil {
		return nil, errors.Trace(err)
	}

	lock, err := s.lock("model-by-name")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	models, ok := controllerModels[controllerName]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controllerName)
	}
	details, ok := models.Models[modelName]
	if !ok {
		return nil, errors.NotFoundf("model %s:%s", controllerName, modelName)
	}
	return &details, nil
}

// RemoveModel implements ModelRemover.
func (s *store) RemoveModel(controllerName, modelName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("remove-model")
	if err != nil {
		return errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	models, ok := controllerModels[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if _, ok := models.Models[modelName]; !ok {
		return errors.NotFoundf("model %s:%s", controllerName, modelName)
	}

	delete(models.Models, modelName)
	if models.CurrentModel == modelName {
		models.CurrentModel = ""
	}
	return errors.Trace(WriteModelsFile(controllerModels))
}
