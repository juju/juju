// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package jujuclient provides functionality to support
// connections to Juju such as controllers cache, accounts cache, etc.

package jujuclient

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/mutex"
	"github.com/juju/utils/clock"

	"github.com/juju/juju/cloud"
)

var _ ClientStore = (*store)(nil)

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

func (s *store) acquireLock() (mutex.Releaser, error) {
	const lockName = "store-lock"
	spec := mutex.Spec{
		Name:    lockName,
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
		return nil, errors.Annotate(err, "cannot read all controllers")
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
		return "", errors.Annotate(err, "cannot get current controller name")
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

// ControllerByName implements ControllersGetter.
func (s *store) ControllerByName(name string) (*ControllerDetails, error) {
	if err := ValidateControllerName(name); err != nil {
		return nil, errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Annotatef(err, "cannot read controller %v", name)
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
		return errors.Annotatef(err, "cannot add controller %v", name)
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
		return errors.Annotatef(err, "cannot update controller %v", name)
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
		return errors.Annotate(err, "cannot set current controller name")
	}
	defer releaser.Release()

	controllers, err := ReadControllersFile(JujuControllersPath())
	if err != nil {
		return errors.Trace(err)
	}
	if _, ok := controllers.Controllers[name]; !ok {
		return errors.NotFoundf("controller %v", name)
	}
	if controllers.CurrentController == name {
		return nil
	}
	controllers.CurrentController = name
	return WriteControllersFile(controllers)
}

// RemoveController implements ControllersRemover
func (s *store) RemoveController(name string) error {
	if err := ValidateControllerName(name); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Annotatef(err, "cannot remove controller %v", name)
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

	// Finally, remove the controllers. This must be done last
	// so we don't end up with dangling entries in other files.
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

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Trace(err)
	}
	defer releaser.Release()

	return errors.Trace(updateModels(
		controllerName,
		func(models *ControllerModels) (bool, error) {
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
func (s *store) SetCurrentModel(controllerName, modelName string) error {
	if err := ValidateControllerName(controllerName); err != nil {
		return errors.Trace(err)
	}
	if err := ValidateModelName(modelName); err != nil {
		return errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return errors.Trace(err)
	}
	defer releaser.Release()

	return errors.Trace(updateModels(
		controllerName,
		func(models *ControllerModels) (bool, error) {
			if models.CurrentModel == modelName {
				return false, nil
			}
			if _, ok := models.Models[modelName]; !ok {
				return false, errors.NotFoundf(
					"model %s:%s",
					controllerName,
					modelName,
				)
			}
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
		return nil, errors.Trace(err)
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
		return "", errors.Trace(err)
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
	if controllerModels.CurrentModel == "" {
		return "", errors.NotFoundf(
			"current model for controller %s",
			controllerName,
		)
	}
	return controllerModels.CurrentModel, nil
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
		return nil, errors.Trace(err)
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
		return errors.Trace(err)
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
			if models.CurrentModel == modelName {
				models.CurrentModel = ""
			}
			return true, nil
		},
	))
}

func updateModels(
	controllerName string,
	update func(*ControllerModels) (bool, error),
) error {
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
		return errors.Trace(err)
	}
	defer releaser.Release()

	accounts, err := ReadAccountsFile(JujuAccountsPath())
	if err != nil {
		return errors.Trace(err)
	}
	if accounts == nil {
		accounts = make(map[string]AccountDetails)
	}
	if oldDetails, ok := accounts[controllerName]; ok && details == oldDetails {
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

// AccountByName implements AccountGetter.
func (s *store) AccountDetails(controllerName string) (*AccountDetails, error) {
	if err := ValidateControllerName(controllerName); err != nil {
		return nil, errors.Trace(err)
	}

	releaser, err := s.acquireLock()
	if err != nil {
		return nil, errors.Trace(err)
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
		return errors.Trace(err)
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
		return errors.Annotatef(err, "cannot update credentials for %v", cloudName)
	}
	defer releaser.Release()

	all, err := ReadCredentialsFile(JujuCredentialsPath())
	if err != nil {
		return errors.Annotate(err, "cannot get credentials")
	}

	if len(all) == 0 {
		all = make(map[string]cloud.CloudCredential)
	}

	// Clear the default credential if we are removing that one.
	if existing, ok := all[cloudName]; ok && existing.DefaultCredential != "" {
		stillHaveDefault := false
		for name := range details.AuthCredentials {
			if name == existing.DefaultCredential {
				stillHaveDefault = true
				break
			}
		}
		if !stillHaveDefault {
			details.DefaultCredential = ""
		}
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
		return errors.Annotatef(err, "cannot update bootstrap config for controller %s", controllerName)
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
	if cfg.CloudType == "" {
		// TODO(axw) 2016-07-25 #1603841
		// Drop this when we get to 2.0. This exists only for
		// compatibility with previous beta releases.
		cfg.CloudType, _ = cfg.Config["type"].(string)
	}
	return &cfg, nil
}
