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

// AllControllers implements ControllersGetter.
func (s *store) AllControllers() (map[string]ControllerDetails, error) {
	lock, err := s.lock("read-all-controllers")
	if err != nil {
		return nil, errors.Annotate(err, "cannot read all controllers")
	}
	defer s.unlock(lock)
	return ReadControllersFile(JujuControllersPath())
}

// ControllerByName implements ControllersGetter.
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

// UpdateController implements ControllersUpdater.
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

// RemoveController implements ControllersRemover
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

	// Remove accounts for the controller.
	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := controllerAccounts[name]; ok {
		delete(controllerAccounts, name)
		if err := WriteAccountsFile(controllerAccounts); err != nil {
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

// SetCurrentModel implements ModelUpdater.
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

// UpdateAccount implements AccountUpdater.
func (s *store) UpdateAccount(controllerName, accountName string, details AccountDetails) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountDetails(details); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("update-account")
	if err != nil {
		return errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if controllerAccounts == nil {
		controllerAccounts = make(map[string]*ControllerAccounts)
	}
	accounts, ok := controllerAccounts[controllerName]
	if !ok {
		accounts = &ControllerAccounts{
			Accounts: make(map[string]AccountDetails),
		}
		controllerAccounts[controllerName] = accounts
	}
	if oldDetails, ok := accounts.Accounts[accountName]; ok && details == oldDetails {
		return nil
	}

	accounts.Accounts[accountName] = details
	return errors.Trace(WriteAccountsFile(controllerAccounts))
}

// SetCurrentAccount implements AccountUpdater.
func (s *store) SetCurrentAccount(controllerName, accountName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("set-current-account")
	if err != nil {
		return errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	accounts, ok := controllerAccounts[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if accounts.CurrentAccount == accountName {
		return nil
	}
	if _, ok := accounts.Accounts[accountName]; !ok {
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
	}

	accounts.CurrentAccount = accountName
	return errors.Trace(WriteAccountsFile(controllerAccounts))
}

// AllAccounts implements AccountGetter.
func (s *store) AllAccounts(controllerName string) (map[string]AccountDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}

	lock, err := s.lock("read-all-accounts")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	accounts, ok := controllerAccounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("accounts for controller %s", controllerName)
	}
	return accounts.Accounts, nil
}

// CurrentAccount implements AccountGetter.
func (s *store) CurrentAccount(controllerName string) (string, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return "", errors.Trace(err)
	}

	lock, err := s.lock("read-current-account")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return "", errors.Trace(err)
	}
	accounts, ok := controllerAccounts[controllerName]
	if !ok || accounts.CurrentAccount == "" {
		return "", errors.NotFoundf("current account for controller %s", controllerName)
	}
	return accounts.CurrentAccount, nil
}

// AccountByName implements AccountGetter.
func (s *store) AccountByName(controllerName, accountName string) (*AccountDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
		return nil, errors.Trace(err)
	}

	lock, err := s.lock("account-by-name")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	accounts, ok := controllerAccounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("controller %s", controllerName)
	}
	details, ok := accounts.Accounts[accountName]
	if !ok {
		return nil, errors.NotFoundf("account %s:%s", controllerName, accountName)
	}
	return &details, nil
}

// RemoveAccount implements AccountRemover.
func (s *store) RemoveAccount(controllerName, accountName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
		return errors.Trace(err)
	}

	lock, err := s.lock("remove-account")
	if err != nil {
		return errors.Trace(err)
	}
	defer s.unlock(lock)

	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	accounts, ok := controllerAccounts[controllerName]
	if !ok {
		return errors.NotFoundf("controller %s", controllerName)
	}
	if _, ok := accounts.Accounts[accountName]; !ok {
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
	}

	delete(accounts.Accounts, accountName)
	if accounts.CurrentAccount == accountName {
		accounts.CurrentAccount = ""
	}
	return errors.Trace(WriteAccountsFile(controllerAccounts))
}
