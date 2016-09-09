// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
)

// ControllerDetails holds the details needed to connect to a controller.
type ControllerDetails struct {
	// UnresolvedAPIEndpoints holds a list of API addresses which may
	// contain unresolved hostnames. It's used to compare more recent
	// API addresses before resolving hostnames to determine if the
	// cached addresses have changed and therefore perform (a possibly
	// slow) local DNS resolution before comparing them against Addresses.
	UnresolvedAPIEndpoints []string `yaml:"unresolved-api-endpoints,flow"`

	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"uuid"`

	// APIEndpoints holds a list of API addresses. It may not be
	// current, and it will be empty if the environment has not been
	// bootstrapped.
	APIEndpoints []string `yaml:"api-endpoints,flow"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert"`

	// Cloud is the name of the cloud that this controller runs in.
	Cloud string `yaml:"cloud"`

	// CloudRegion is the name of the cloud region that this controller
	// runs in. This will be empty for clouds without regions.
	CloudRegion string `yaml:"region,omitempty"`

	// AgentVersion is the version of the agent running on this controller.
	// While this isn't strictly needed to connect to a controller, it is used
	// in formatting show-controller output where this struct is also used.
	AgentVersion string `yaml:"agent-version,omitempty"`

	// ControllerMachineCount represents the number of controller machines
	// It is cached here so under normal usage list-controllers
	// does not need to hit the server.
	ControllerMachineCount int `yaml:"controller-machine-count"`

	// ActiveControllerMachineCount represents the number of controller machines
	// and which are active in the HA cluster.
	// It is cached here so under normal usage list-controllers
	// does not need to hit the server.
	ActiveControllerMachineCount int `yaml:"active-controller-machine-count"`

	// ModelCount is the number of models to which a user has access.
	// It is cached here so under normal usage list-controllers
	// does not need to hit the server.
	ModelCount *int `yaml:"model-count,omitempty"`

	// MachineCount is the number of machines in all models to
	// which a user has access. It is cached here so under normal
	// usage list-controllers does not need to hit the server.
	MachineCount *int `yaml:"machine-count,omitempty"`
}

// ModelDetails holds details of a model.
type ModelDetails struct {
	// ModelUUID is the unique ID for the model.
	ModelUUID string `yaml:"uuid"`
}

// AccountDetails holds details of an account.
type AccountDetails struct {
	// User is the username for the account.
	User string `yaml:"user"`

	// Password is the password for the account.
	Password string `yaml:"password,omitempty"`

	// LastKnownAccess is the last known access level for the account.
	LastKnownAccess string `yaml:"last-known-access,omitempty"`
}

// BootstrapConfig holds the configuration used to bootstrap a controller.
//
// This includes all non-sensitive information required to regenerate the
// bootstrap configuration. A reference to the credential used will be
// stored, rather than the credential itself.
type BootstrapConfig struct {
	// ControllerConfig is the controller configuration.
	ControllerConfig controller.Config `yaml:"controller-config"`

	// Config is the complete configuration for the provider.
	Config map[string]interface{} `yaml:"model-config"`

	// ControllerModelUUID is the controller model UUID.
	ControllerModelUUID string `yaml:"controller-model-uuid"`

	// Credential is the name of the credential used to bootstrap.
	//
	// This will be empty if an auto-detected credential was used.
	Credential string `yaml:"credential,omitempty"`

	// Cloud is the name of the cloud to create the Juju controller in.
	Cloud string `yaml:"cloud"`

	// CloudType is the type of the cloud to create the Juju controller in.
	CloudType string `yaml:"type"`

	// CloudRegion is the name of the region of the cloud to create
	// the Juju controller in. This will be empty for clouds without
	// regions.
	CloudRegion string `yaml:"region,omitempty"`

	// CloudEndpoint is the location of the primary API endpoint to
	// use when communicating with the cloud.
	CloudEndpoint string `yaml:"endpoint,omitempty"`

	// CloudIdentityEndpoint is the location of the API endpoint to use
	// when communicating with the cloud's identity service. This will
	// be empty for clouds that have no identity-specific API endpoint.
	CloudIdentityEndpoint string `yaml:"identity-endpoint,omitempty"`

	// CloudStorageEndpoint is the location of the API endpoint to use
	// when communicating with the cloud's storage service. This will
	// be empty for clouds that have no storage-specific API endpoint.
	CloudStorageEndpoint string `yaml:"storage-endpoint,omitempty"`
}

// ControllerUpdater stores controller details.
type ControllerUpdater interface {
	// AddController adds the given controller to the controller
	// collection.
	//
	// Where UpdateController is concerned with the controller name,
	// AddController uses the controller UUID and will not add a
	// duplicate even if the name is different.
	AddController(controllerName string, details ControllerDetails) error

	// UpdateController updates the given controller in the controller
	// collection.
	//
	// If a controller of controllerName exists it will be overwritten
	// with the new details.
	UpdateController(controllerName string, details ControllerDetails) error

	// SetCurrentController sets the name of the current controller.
	// If there exists no controller with the specified name, an error
	// satisfying errors.IsNotFound will be returned.
	SetCurrentController(controllerName string) error
}

// ControllerRemover removes controllers.
type ControllerRemover interface {
	// RemoveController removes the controller with the given name from the
	// controllers collection. Any other controllers with matching UUIDs
	// will also be removed.
	//
	// Removing controllers will remove all information related to those
	// controllers (models, accounts, bootstrap config.)
	RemoveController(controllerName string) error
}

// ControllerGetter gets controllers.
type ControllerGetter interface {
	// AllControllers gets all controllers.
	AllControllers() (map[string]ControllerDetails, error)

	// ControllerByName returns the controller with the specified name.
	// If there exists no controller with the specified name, an error
	// satisfying errors.IsNotFound will be returned.
	ControllerByName(controllerName string) (*ControllerDetails, error)

	// CurrentController returns the name of the current controller.
	// If there is no current controller, an error satisfying
	// errors.IsNotFound will be returned.
	CurrentController() (string, error)
}

// ModelUpdater stores model details.
type ModelUpdater interface {
	// UpdateModel adds the given model to the model collection.
	//
	// If the model does not already exist, it will be added.
	// Otherwise, it will be overwritten with the new details.
	UpdateModel(controllerName, modelName string, details ModelDetails) error

	// SetCurrentModel sets the name of the current model for
	// the specified controller and account. If there exists no
	// model with the specified names, an error satisfying
	// errors.IsNotFound will be returned.
	SetCurrentModel(controllerName, modelName string) error
}

// ModelRemover removes models.
type ModelRemover interface {
	// RemoveModel removes the model with the given controller, account,
	// and model names from the models collection. If there is no model
	// with the specified names, an errors satisfying errors.IsNotFound
	// will be returned.
	RemoveModel(controllerName, modelName string) error
}

// ModelGetter gets models.
type ModelGetter interface {
	// AllModels gets all models for the specified controller as a map
	// from model name to its details.
	//
	// If there is no controller with the specified
	// name, or no models cached for the controller and account,
	// an error satisfying errors.IsNotFound will be returned.
	AllModels(controllerName string) (map[string]ModelDetails, error)

	// CurrentModel returns the name of the current model for
	// the specified controller. If there is no current
	// model for the controller, an error satisfying
	// errors.IsNotFound is returned.
	CurrentModel(controllerName string) (string, error)

	// ModelByName returns the model with the specified controller,
	// and model name. If a model with the specified name does not
	// exist, an error satisfying errors.IsNotFound will be
	// returned.
	ModelByName(controllerName, modelName string) (*ModelDetails, error)
}

// AccountUpdater stores account details.
type AccountUpdater interface {
	// UpdateAccount updates the account associated with the
	// given controller.
	UpdateAccount(controllerName string, details AccountDetails) error
}

// AccountRemover removes accounts.
type AccountRemover interface {
	// RemoveAccount removes the account associated with the given controller.
	// If there is no associated account with the
	// specified names, an errors satisfying errors.IsNotFound will be
	// returned.
	RemoveAccount(controllerName string) error
}

// AccountGetter gets accounts.
type AccountGetter interface {
	// AccountByName returns the account associated with the given
	// controller name. If no associated account exists, an error
	// satisfying errors.IsNotFound will be returned.
	AccountDetails(controllerName string) (*AccountDetails, error)
}

// CredentialGetter gets credentials.
type CredentialGetter interface {
	// CredentialForCloud gets credentials for the named cloud.
	CredentialForCloud(string) (*cloud.CloudCredential, error)

	// AllCredentials gets all credentials.
	AllCredentials() (map[string]cloud.CloudCredential, error)
}

// CredentialUpdater stores credentials.
type CredentialUpdater interface {
	// UpdateCredential adds the given credentials to the credentials
	// collection.
	//
	// If the cloud or credential name does not already exist, it will be added.
	// Otherwise, it will be overwritten with the new details.
	UpdateCredential(cloudName string, details cloud.CloudCredential) error
}

// BootstrapConfigUpdater stores bootstrap config.
type BootstrapConfigUpdater interface {
	// UpdateBootstrapConfig adds the given bootstrap config to the
	// bootstrap config collection for the controller with the given
	// name.
	//
	// If the bootstrap config does not already exist, it will be added.
	// Otherwise, it will be overwritten with the new value.
	UpdateBootstrapConfig(controller string, cfg BootstrapConfig) error
}

// BootstrapConfigGetter gets bootstrap config.
type BootstrapConfigGetter interface {
	// BootstrapConfigForController gets bootstrap config for the named
	// controller.
	BootstrapConfigForController(string) (*BootstrapConfig, error)
}

// ControllerStore is an amalgamation of ControllerUpdater, ControllerRemover,
// and ControllerGetter.
type ControllerStore interface {
	ControllerUpdater
	ControllerRemover
	ControllerGetter
}

// ModelStore is an amalgamation of ModelUpdater, ModelRemover, and ModelGetter.
type ModelStore interface {
	ModelUpdater
	ModelRemover
	ModelGetter
}

// AccountStore is an amalgamation of AccountUpdater, AccountRemover, and AccountGetter.
type AccountStore interface {
	AccountUpdater
	AccountRemover
	AccountGetter
}

// CredentialStore is an amalgamation of CredentialsUpdater, and CredentialsGetter.
type CredentialStore interface {
	CredentialGetter
	CredentialUpdater
}

// BootstrapConfigStore is an amalgamation of BootstrapConfigUpdater and
// BootstrapConfigGetter.
type BootstrapConfigStore interface {
	BootstrapConfigUpdater
	BootstrapConfigGetter
}

// ClientStore is an amalgamation of AccountStore, BootstrapConfigStore,
// ControllerStore, CredentialStore, and ModelStore.
type ClientStore interface {
	AccountStore
	BootstrapConfigStore
	ControllerStore
	CredentialStore
	ModelStore
}
