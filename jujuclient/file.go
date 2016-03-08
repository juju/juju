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

	"github.com/juju/juju/cloud"
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

// NewFileCredentialStore returns a new filesystem-based credentials store
// that manages credentials in $XDG_DATA_HOME/juju.
func NewFileCredentialStore() CredentialStore {
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
func (s *store) UpdateModel(controllerName, accountName, modelName string, details ModelDetails) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
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

	return errors.Trace(updateAccountModels(
		controllerName, accountName,
		func(models *AccountModels) (bool, error) {
			oldDetails, ok := models.Models[modelName]
			if ok && details == oldDetails {
				return false, nil
			}
			models.Models[modelName] = details
			return true, nil
		},
	))
}

// SetCurrentModel implements ModelUpdater.
func (s *store) SetCurrentModel(controllerName, accountName, modelName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
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

	return errors.Trace(updateAccountModels(
		controllerName, accountName,
		func(models *AccountModels) (bool, error) {
			if models.CurrentModel == modelName {
				return false, nil
			}
			if _, ok := models.Models[modelName]; !ok {
				return false, errors.NotFoundf(
					"model %s:%s:%s",
					controllerName,
					accountName,
					modelName,
				)
			}
			models.CurrentModel = modelName
			return true, nil
		},
	))
}

// AllModels implements ModelGetter.
func (s *store) AllModels(controllerName, accountName string) (map[string]ModelDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
		return nil, errors.Trace(err)
	}

	lock, err := s.lock("read-all-models")
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer s.unlock(lock)

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerAccountModels, ok := all[controllerName]
	if !ok {
		return nil, errors.NotFoundf(
			"models for controller %s",
			controllerName,
		)
	}
	accountModels, ok := controllerAccountModels.AccountModels[accountName]
	if !ok {
		return nil, errors.NotFoundf(
			"models for account %s on controller %s",
			accountName, controllerName,
		)
	}
	return accountModels.Models, nil
}

// CurrentModel implements ModelGetter.
func (s *store) CurrentModel(controllerName, accountName string) (string, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return "", errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
		return "", errors.Trace(err)
	}

	lock, err := s.lock("read-current-model")
	if err != nil {
		return "", errors.Trace(err)
	}
	defer s.unlock(lock)

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return "", errors.Trace(err)
	}
	controllerAccountModels, ok := all[controllerName]
	if !ok {
		return "", errors.NotFoundf(
			"current model for controller %s",
			controllerName,
		)
	}
	accountModels, ok := controllerAccountModels.AccountModels[accountName]
	if !ok || accountModels.CurrentModel == "" {
		return "", errors.NotFoundf(
			"current model for account %s on controller %s",
			accountName, controllerName,
		)
	}
	return accountModels.CurrentModel, nil
}

// ModelByName implements ModelGetter.
func (s *store) ModelByName(controllerName, accountName, modelName string) (*ModelDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
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

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerAccountModels, ok := all[controllerName]
	if !ok {
		return nil, errors.NotFoundf(
			"models for controller %s",
			controllerName,
		)
	}
	accountModels, ok := controllerAccountModels.AccountModels[accountName]
	if !ok {
		return nil, errors.NotFoundf(
			"models for account %s on controller %s",
			accountName, controllerName,
		)
	}
	details, ok := accountModels.Models[modelName]
	if !ok {
		return nil, errors.NotFoundf(
			"model %s:%s:%s",
			controllerName,
			accountName,
			modelName,
		)
	}
	return &details, nil
}

// RemoveModel implements ModelRemover.
func (s *store) RemoveModel(controllerName, accountName, modelName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountName(accountName); err != nil {
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

	return errors.Trace(updateAccountModels(
		controllerName, accountName,
		func(models *AccountModels) (bool, error) {
			if _, ok := models.Models[modelName]; !ok {
				return false, errors.NotFoundf(
					"model %s:%s:%s",
					controllerName,
					accountName,
					modelName,
				)
			}
			delete(models.Models, modelName)
			if models.CurrentModel == modelName {
				models.CurrentModel = modelName
			}
			return true, nil
		},
	))
}

func updateAccountModels(
	controllerName, accountName string,
	update func(*AccountModels) (bool, error),
) error {
	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if all == nil {
		all = make(map[string]ControllerAccountModels)
	}
	controllerAccountModels, ok := all[controllerName]
	if !ok {
		controllerAccountModels = ControllerAccountModels{
			make(map[string]*AccountModels),
		}
		all[controllerName] = controllerAccountModels
	}
	accountModels, ok := controllerAccountModels.AccountModels[accountName]
	if !ok {
		accountModels = &AccountModels{
			Models: make(map[string]ModelDetails),
		}
		controllerAccountModels.AccountModels[accountName] = accountModels
	}
	updated, err := update(accountModels)
	if err != nil {
		return errors.Trace(err)
	}
	if updated {
		return errors.Trace(WriteModelsFile(all))
	}
	return nil
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
		return errors.NotFoundf("account %s:%s", controllerName, accountName)
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

// UpdateCredential implements CredentialUpdater.
func (s *store) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	lock, err := s.lock("update-credentials")
	if err != nil {
		return errors.Annotatef(err, "cannot update credentials for %v", cloudName)
	}
	defer s.unlock(lock)

	all, err := ReadCredentialsFile(JujuCredentialsPath())
	if err != nil {
		return errors.Annotate(err, "cannot get credentials")
	}

	if len(all) == 0 {
		all = make(map[string]cloud.CloudCredential)
	}

	all[cloudName] = details
	return WriteCredentialsFile(all)
}

// CredentialForCloud implements CredentialGetter.
func (s *store) CredentialForCloud(cloudName string) (*cloud.CloudCredential, error) {
	cloudCredentials, err := s.AllCredentials()
	if err != nil {
		return nil, errors.Trace(err)
	}
	credentials, ok := cloudCredentials[cloudName]
	if !ok {
		return nil, errors.NotFoundf("credentials for cloud %s", cloudName)
	}
	return &credentials, nil
}

// AllCredentials implements CredentialGetter.
func (s *store) AllCredentials() (map[string]cloud.CloudCredential, error) {
	cloudCredentials, err := ReadCredentialsFile(JujuCredentialsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return cloudCredentials, nil
}
