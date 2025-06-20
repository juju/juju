// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jujuclient provides functionality to support
// connections to Juju such as controllers cache, accounts cache, etc.

package jujuclient

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/mutex/v2"
	cookiejar "github.com/juju/persistent-cookiejar"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/juju/osenv"
)

var (
	_ ClientStore = (*store)(nil)

	// A second should be enough to write or read any files. But
	// some disks are slow when under load, so lets give the disk a
	// reasonable time to get the lock.
	lockTimeout = 5 * time.Second
)

// NewFileClientStore returns a new filesystem-based client store
// that manages files in $XDG_DATA_HOME/juju.
func NewFileClientStore() ClientStore {
	return &store{
		lockName: generateStoreLockName(),
	}
}

// NewFileCredentialStore returns a new filesystem-based credentials store
// that manages credentials in $XDG_DATA_HOME/juju.
func NewFileCredentialStore() CredentialStore {
	return &store{
		lockName: generateStoreLockName(),
	}
}

type store struct {
	lockName string
}

// generateStoreLockName uses part of the hash of the controller path as the
// name of the lock. This is to avoid contention between multiple users on a
// single machine with different controller files, but also helps with
// contention in tests.
func generateStoreLockName() string {
	h := sha256.New()
	_, _ = h.Write([]byte(JujuControllersPath()))
	fullHash := fmt.Sprintf("%x", h.Sum(nil))
	return fmt.Sprintf("store-lock-%x", fullHash[:8])
}

func (s *store) acquireLock() (mutex.Releaser, error) {
	spec := mutex.Spec{
		Name:    s.lockName,
		Clock:   clock.WallClock,
		Delay:   20 * time.Millisecond,
		Timeout: lockTimeout,
	}
	releaser, err := mutex.Acquire(spec)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return releaser, nil
}

// AllControllers implements ControllersGetter.
func (s *store) AllControllers() (map[string]ControllerDetails, error) {
	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Annotate(err,
			"cannot acquire lock file to read all the controllers",
		)
	}
	defer releaser.Release()
	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return controllers.Controllers, nil
}

// CurrentController implements ControllersGetter.
func (s *store) CurrentController() (string, error) {
	releaser, err := s.acquireLock()
	if err != nil {
		return "", errors.Annotate(err,
			"cannot acquire lock file to get the current controller name",
		)
	}
	defer releaser.Release()
	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return "", errors.Trace(err)
	}
	if controllers.CurrentController == "" {
		return "", errors.NotFoundf("current controller")
	}
	return controllers.CurrentController, nil
}

// PreviousController implements ControllerGetter
func (s *store) PreviousController() (string, bool, error) {
	releaser, err := s.acquireLock()
	if err != nil {
		return "", false, errors.Annotate(err,
			"cannot acquire lock file to get the previous controller name",
		)
	}
	defer releaser.Release()
	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return "", false, errors.Trace(err)
	}
	if controllers.PreviousController == "" {
		return "", false, errors.NotFoundf("previous controller")
	}
	return controllers.PreviousController, controllers.HasControllerChangedOnPreviousSwitch, nil
}

// ControllerByName implements ControllersGetter.
func (s *store) ControllerByName(name string) (*ControllerDetails, error) {
	if err := ValidateControllerName(name); err != nil {
		return nil, errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Annotatef(err,
			"cannot acquire lock file to read controller %s", name,
		)
	}
	defer releaser.Release()

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	if result, ok := controllers.Controllers[name]; ok {
		return &result, nil
	}
	return nil, errors.NotFoundf("controller %s", name)
}

// ControllerByEndpoints implements ControllersGetter.
func (s *store) ControllerByAPIEndpoints(endpoints ...string) (*ControllerDetails, string, error) {
	releaser, err := s.acquireLock()
	if err != nil {
		return nil, "", errors.Annotatef(err, "cannot acquire lock file to read controllers")
	}
	defer releaser.Release()

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return nil, "", errors.Trace(err)
	}

	matchEps := set.NewStrings(endpoints...)
	for name, ctrl := range controllers.Controllers {
		if matchEps.Intersection(set.NewStrings(ctrl.APIEndpoints...)).IsEmpty() {
			continue
		}

		return &ctrl, name, nil
	}
	return nil, "", errors.NotFoundf("controller with API endpoints %v", endpoints)
}

// AddController implements ControllerUpdater.
func (s *store) AddController(name string, details ControllerDetails) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateControllerDetails(details); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file to add controller %s", name,
		)
	}
	defer releaser.Release()

	all, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	if len(all.Controllers) == 0 {
		all.Controllers = make(map[string]ControllerDetails)
	}

	if _, ok := all.Controllers[name]; ok {
		return errors.AlreadyExistsf("controller with name %s", name)
	}

	for k, v := range all.Controllers {
		if v.ControllerUUID == details.ControllerUUID {
			return errors.AlreadyExistsf("controller with UUID %s (%s)",
				details.ControllerUUID, k)
		}
	}

	all.Controllers[name] = details
	return WriteControllersFile(all)
}

// UpdateController implements ControllerUpdater.
func (s *store) UpdateController(name string, details ControllerDetails) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateControllerDetails(details); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file to update controller %s", name,
		)
	}
	defer releaser.Release()

	all, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	if len(all.Controllers) == 0 {
		return errors.NotFoundf("controllers")
	}

	for k, v := range all.Controllers {
		if v.ControllerUUID == details.ControllerUUID && k != name {
			return errors.AlreadyExistsf("controller %s with UUID %s",
				k, v.ControllerUUID)
		}
	}

	if _, ok := all.Controllers[name]; !ok {
		return errors.NotFoundf("controller %s", name)
	}

	all.Controllers[name] = details
	return WriteControllersFile(all)
}

// SetCurrentController implements ControllerUpdater.
func (s *store) SetCurrentController(name string) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotate(err,
			"cannot acquire lock file to set the current controller name",
		)
	}
	defer releaser.Release()

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := controllers.Controllers[name]; !ok {
		return errors.NotFoundf("controller %v", name)
	}
	controllers.HasControllerChangedOnPreviousSwitch = controllers.CurrentController != name
	if controllers.HasControllerChangedOnPreviousSwitch {
		controllers.PreviousController = controllers.CurrentController
		controllers.CurrentController = name
	}
	return WriteControllersFile(controllers)
}

// RemoveController implements ControllersRemover
func (s *store) RemoveController(name string) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file to remove controller %s", name,
		)
	}
	defer releaser.Release()

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Annotate(err, "cannot get controllers")
	}

	// We remove all controllers with the same UUID as the named one.
	namedControllerDetails, ok := controllers.Controllers[name]
	if !ok {
		return nil
	}
	var names []string
	for name, details := range controllers.Controllers {
		if details.ControllerUUID == namedControllerDetails.ControllerUUID {
			names = append(names, name)
			delete(controllers.Controllers, name)
			if controllers.CurrentController == name {
				controllers.CurrentController = ""
			}
		}
	}

	// Remove models for the controller.
	controllerModels, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	for _, name := range names {
		if _, ok := controllerModels[name]; ok {
			delete(controllerModels, name)
			if err := WriteModelsFile(controllerModels); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Remove accounts for the controller.
	controllerAccounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	for _, name := range names {
		if _, ok := controllerAccounts[name]; ok {
			delete(controllerAccounts, name)
			if err := WriteAccountsFile(controllerAccounts); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Remove bootstrap config for the controller.
	bootstrapConfigurations, err := ReadBootstrapConfigFile(JujuBootstrapConfigPath())
	if err != nil {
		return errors.Trace(err)
	}
	for _, name := range names {
		if _, ok := bootstrapConfigurations[name]; ok {
			delete(bootstrapConfigurations, name)
			if err := WriteBootstrapConfigFile(bootstrapConfigurations); err != nil {
				return errors.Trace(err)
			}
		}
	}

	// Remove the controller cookie jars.
	for _, name := range names {
		err := os.Remove(JujuCookiePath(name))
		if err != nil && !os.IsNotExist(err) {
			return errors.Trace(err)
		}
	}

	// Finally, remove the controllers. This must be done last
	// so we don't end up with dangling entries in other files.
	return WriteControllersFile(controllers)
}

// UpdateModel implements ModelUpdater.
func (s *store) UpdateModel(controllerName, modelName string, details ModelDetails) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateModel(modelName, details); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for updating model %s on controller %s", modelName, controllerName,
		)
	}
	defer releaser.Release()

	return errors.Trace(updateModels(
		controllerName,
		func(models *ControllerModels) (bool, error) {
			oldDetails, ok := models.Models[modelName]
			if ok && details == oldDetails {
				return false, nil
			}
			if ok && oldDetails.ModelType == "" && details.ModelType != model.IAAS ||
				oldDetails.ModelType != "" && oldDetails.ModelType != details.ModelType {
				oldModelType := oldDetails.ModelType
				if oldModelType == "" {
					oldModelType = model.IAAS
				}
				return false, errors.Errorf(
					"model type was %q, cannot change to %q", oldModelType, details.ModelType)
			}
			models.Models[modelName] = details
			return true, nil
		},
	))
}

// SetCurrentModel implements ModelUpdater.
func (s *store) SetCurrentModel(controllerName, modelName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for setting current model %s on controller %s", modelName, controllerName,
		)
	}
	defer releaser.Release()

	if modelName != "" {
		if err := ValidateModelName(modelName); err != nil {
			return errors.Trace(err)
		}
	}
	return errors.Trace(updateModels(
		controllerName,
		func(models *ControllerModels) (bool, error) {
			if modelName == "" {
				// We just want to reset
				models.CurrentModel = ""
				return true, nil
			}
			if _, ok := models.Models[modelName]; !ok {
				return false, errors.NotFoundf(
					"model %s:%s",
					controllerName,
					modelName,
				)
			}
			models.PreviousModel = models.CurrentModel
			models.CurrentModel = modelName
			return true, nil
		},
	))
}

// AllModels implements ModelGetter.
func (s *store) AllModels(controllerName string) (map[string]ModelDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Annotatef(err,
			"cannot acquire lock file for getting all models for controller %s", controllerName,
		)
	}
	defer releaser.Release()

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerModels, ok := all[controllerName]
	if !ok {
		return nil, errors.NotFoundf(
			"models for controller %s",
			controllerName,
		)
	}
	return controllerModels.Models, nil
}

// CurrentModel implements ModelGetter.
func (s *store) CurrentModel(controllerName string) (string, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return "", errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return "", errors.Annotatef(err,
			"cannot acquire lock file for getting current model for controller %s", controllerName,
		)
	}
	defer releaser.Release()

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return "", errors.Trace(err)
	}
	controllerModels, ok := all[controllerName]
	if !ok {
		return "", errors.NotFoundf(
			"current model for controller %s",
			controllerName,
		)
	}

	var controller bool
	var modelNames []string
	for ns := range controllerModels.Models {
		name, _, err := SplitFullyQualifiedModelName(ns)
		if err != nil {
			continue
		}
		if name == "controller" {
			controller = true
			continue
		}
		modelNames = append(modelNames, controllerName+":"+ns)
	}

	if controllerModels.CurrentModel == "" {
		num := len(modelNames)
		if num == 0 {
			if !controller {
				return "", errors.NotFoundf(
					"current model for controller %s",
					controllerName,
				)
			}
			return "", errors.NewNotFound(nil, `No selected model.

Only the controller model exists. Use "juju add-model" to create an initial model.
`)
		}

		msg := `No selected model.

Use "juju switch" to select one of the following models:

  - ` + strings.Join(modelNames, "\n  - ")
		return "", errors.NewNotFound(nil, msg)
	}
	return controllerModels.CurrentModel, nil
}

// PreviousModel implements ModelGetter.
func (s *store) PreviousModel(controllerName string) (string, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return "", errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return "", errors.Annotatef(err,
			"cannot acquire lock file for getting current model for controller %s", controllerName,
		)
	}
	defer releaser.Release()

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return "", errors.Trace(err)
	}
	controllerModels, ok := all[controllerName]
	if !ok {
		return "", errors.NotFoundf(
			"previous model for controller %s",
			controllerName,
		)
	}

	if controllerModels.PreviousModel == "" {
		return "", errors.NotFoundf("previous model for controller %s", controllerName)
	}
	return controllerModels.PreviousModel, nil
}

// ModelByName implements ModelGetter.
func (s *store) ModelByName(controllerName, modelName string) (*ModelDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	if err := ValidateModelName(modelName); err != nil {
		return nil, errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Annotatef(err,
			"cannot acquire lock file for getting model %s for controller %s", modelName, controllerName,
		)
	}
	defer releaser.Release()

	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	controllerModels, ok := all[controllerName]
	if !ok {
		return nil, errors.NotFoundf(
			"model %s:%s",
			controllerName,
			modelName,
		)
	}
	details, ok := controllerModels.Models[modelName]
	if !ok {
		return nil, errors.NotFoundf(
			"model %s:%s",
			controllerName,
			modelName,
		)
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

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for removing model %s on controller %s", modelName, controllerName,
		)
	}
	defer releaser.Release()

	return errors.Trace(updateModels(
		controllerName,
		func(models *ControllerModels) (bool, error) {
			if _, ok := models.Models[modelName]; !ok {
				return false, errors.NotFoundf(
					"model %s:%s",
					controllerName,
					modelName,
				)
			}
			delete(models.Models, modelName)
			return true, nil
		},
	))
}

type updateModelFunc func(storedModels *ControllerModels) (bool, error)

func updateModels(controllerName string, update updateModelFunc) error {
	all, err := ReadModelsFile(JujuModelsPath())
	if err != nil {
		return errors.Trace(err)
	}
	controllerModels, ok := all[controllerName]
	if !ok {
		if all == nil {
			all = make(map[string]*ControllerModels)
		}
		controllerModels = &ControllerModels{}
		all[controllerName] = controllerModels
	}
	if controllerModels.Models == nil {
		controllerModels.Models = make(map[string]ModelDetails)
	}
	updated, err := update(controllerModels)
	if err != nil {
		return errors.Trace(err)
	}
	if updated {
		return errors.Trace(WriteModelsFile(all))
	}
	return nil
}

// SetModels implements ModelUpdater.
func (s *store) SetModels(controllerName string, models map[string]ModelDetails) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}

	for modelName, details := range models {
		if err := ValidateModel(modelName, details); err != nil {
			return errors.Trace(err)
		}
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for setting models on controller %s", controllerName,
		)
	}
	defer releaser.Release()

	err = updateModels(controllerName, func(storedModels *ControllerModels) (bool, error) {
		changed := len(storedModels.Models) != len(models)
		// Add or update controller models based on a new collection.
		for modelName, details := range models {
			oldDetails, ok := storedModels.Models[modelName]
			if ok && details == oldDetails {
				continue
			}
			storedModels.Models[modelName] = details
			changed = true
		}
		// Delete models that are not in the new collection.
		for modelName := range storedModels.Models {
			if _, ok := models[modelName]; !ok {
				delete(storedModels.Models, modelName)
			}
		}
		return changed, nil
	})
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}

// UpdateAccount implements AccountUpdater.
func (s *store) UpdateAccount(controllerName string, details AccountDetails) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateAccountDetails(details); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for updating an account on controller %s", controllerName,
		)
	}
	defer releaser.Release()

	accounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if accounts == nil {
		accounts = make(map[string]AccountDetails)
	}
	if oldDetails, ok := accounts[controllerName]; ok && reflect.DeepEqual(details, oldDetails) {
		return nil
	} else {
		// Only update last known access if it has a value.
		if details.LastKnownAccess == "" {
			details.LastKnownAccess = oldDetails.LastKnownAccess
		}
	}

	accounts[controllerName] = details
	return errors.Trace(WriteAccountsFile(accounts))
}

// AccountDetails implements AccountGetter.
func (s *store) AccountDetails(controllerName string) (*AccountDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Annotatef(err,
			"cannot acquire lock file for getting an account details on controller %s", controllerName,
		)
	}
	defer releaser.Release()

	accounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	details, ok := accounts[controllerName]
	if !ok {
		return nil, errors.NotFoundf("account details for controller %s", controllerName)
	}
	return &details, nil
}

// RemoveAccount implements AccountRemover.
func (s *store) RemoveAccount(controllerName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for removing an account on controller %s", controllerName,
		)
	}
	defer releaser.Release()

	accounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := accounts[controllerName]; !ok {
		return errors.NotFoundf("account details for controller %s", controllerName)
	}

	delete(accounts, controllerName)
	return errors.Trace(WriteAccountsFile(accounts))
}

// UpdateCredential implements CredentialUpdater.
func (s *store) UpdateCredential(cloudName string, details cloud.CloudCredential) error {
	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for updating credentials for %s", cloudName,
		)
	}
	defer releaser.Release()

	credentials, err := ReadCredentialsFile(JujuCredentialsPath())
	if err != nil {
		return errors.Annotate(err, "cannot get credentials")
	}

	credentials.UpdateCloudCredential(cloudName, details)
	return WriteCredentialsFile(credentials)
}

// CredentialForCloud implements CredentialGetter.
func (s *store) CredentialForCloud(cloudName string) (*cloud.CloudCredential, error) {
	credentialCollection, err := ReadCredentialsFile(JujuCredentialsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	credential, err := credentialCollection.CloudCredential(cloudName)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return credential, nil
}

// AllCredentials implements CredentialGetter.
func (s *store) AllCredentials() (map[string]cloud.CloudCredential, error) {
	credentialCollection, err := ReadCredentialsFile(JujuCredentialsPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	cloudNames := credentialCollection.CloudNames()
	cloudCredentials := make(map[string]cloud.CloudCredential)
	for _, cloudName := range cloudNames {
		v, err := credentialCollection.CloudCredential(cloudName)
		if err != nil {
			return nil, errors.Trace(err)
		}
		cloudCredentials[cloudName] = *v
	}
	return cloudCredentials, nil
}

// UpdateBootstrapConfig implements BootstrapConfigUpdater.
func (s *store) UpdateBootstrapConfig(controllerName string, cfg BootstrapConfig) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateBootstrapConfig(cfg); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err,
			"cannot acquire lock file for updating the bootstrap config for controller %s", controllerName,
		)
	}
	defer releaser.Release()

	all, err := ReadBootstrapConfigFile(JujuBootstrapConfigPath())
	if err != nil {
		return errors.Annotate(err, "cannot get bootstrap config")
	}

	if all == nil {
		all = make(map[string]BootstrapConfig)
	}
	all[controllerName] = cfg
	return WriteBootstrapConfigFile(all)
}

// BootstrapConfigForController implements BootstrapConfigGetter.
func (s *store) BootstrapConfigForController(controllerName string) (*BootstrapConfig, error) {
	configs, err := ReadBootstrapConfigFile(JujuBootstrapConfigPath())
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg, ok := configs[controllerName]
	if !ok {
		return nil, errors.NotFoundf("bootstrap config for controller %s", controllerName)
	}
	return &cfg, nil
}

// CookieJar returns the cookie jar associated with the given controller.
func (s *store) CookieJar(controllerName string) (CookieJar, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}
	path := JujuCookiePath(controllerName)
	jar, err := cookiejar.New(&cookiejar.Options{
		Filename: path,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &cookieJar{
		path: path,
		Jar:  jar,
	}, nil
}

type cookieJar struct {
	path string
	*cookiejar.Jar
}

func (jar *cookieJar) Save() error {
	// Ensure that the directory exists before saving.
	if err := os.MkdirAll(filepath.Dir(jar.path), 0700); err != nil {
		return errors.Annotatef(err, "cannot make cookies directory")
	}
	return jar.Jar.Save()
}

// JujuCookiePath is the location where cookies associated
// with the given controller are expected to be found.
func JujuCookiePath(controllerName string) string {
	return osenv.JujuXDGDataHomePath("cookies", controllerName+".json")
}
