// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import "github.com/juju/juju/cloud"

// ControllerDetails holds the details needed to connect to a controller.
type ControllerDetails struct {
	// Servers contains the addresses of hosts that form the Juju controller
	// cluster.
	Servers []string `yaml:"servers,flow"`

	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"uuid"`

	// APIEndpoints is the collection of API endpoints running in this controller.
	APIEndpoints []string `yaml:"api-endpoints,flow"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert"`
}

// ModelDetails holds details of a model.
type ModelDetails struct {
	// ModelUUID holds the details of a model.
	ModelUUID string `yaml:"uuid"`
}

// AccountDetails holds details of an account.
type AccountDetails struct {
	// User is the username for the account.
	User string `yaml:"user"`

	// Password is the password for the account.
	Password string `yaml:"password,omitempty"`
}

// ControllerUpdater stores controller details.
type ControllerUpdater interface {
	// UpdateController adds the given controller to the controller
	// collection.
	//
	// If the controller does not already exist, it will be added.
	// Otherwise, it will be overwritten with the new details.
	UpdateController(controllerName string, details ControllerDetails) error
}

// ControllerRemover removes controllers.
type ControllerRemover interface {
	// RemoveController removes the controller with the given name from the
	// controllers collection.
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
}

// ModelUpdater stores model details.
type ModelUpdater interface {
	// UpdateModel adds the given model to the model collection.
	//
	// If the model does not already exist, it will be added.
	// Otherwise, it will be overwritten with the new details.
	UpdateModel(controllerName, accountName, modelName string, details ModelDetails) error

	// SetCurrentModel sets the name of the current model for
	// the specified controller and account. If there exists no
	// model with the specified names, an error satisfing
	// errors.IsNotFound will be returned.
	SetCurrentModel(controllerName, accountName, modelName string) error
}

// ModelRemover removes models.
type ModelRemover interface {
	// RemoveModel removes the model with the given controller, account,
	// and model names from the models collection. If there is no model
	// with the specified names, an errors satisfying errors.IsNotFound
	// will be returned.
	RemoveModel(controllerName, accountName, modelName string) error
}

// ModelGetter gets models.
type ModelGetter interface {
	// AllModels gets all models for the specified controller and
	// account.
	//
	// If there is no controller or account with the specified
	// names, or no models cached for the controller and account,
	// an error satisfying errors.IsNotFound will be returned.
	AllModels(controllerName, accountName string) (map[string]ModelDetails, error)

	// CurrentModel returns the name of the current model for
	// the specified controller and account. If there is no current
	// model for the controller and account, an error satisfying
	// errors.IsNotFound is returned.
	CurrentModel(controllerName, accountName string) (string, error)

	// ModelByName returns the model with the specified controller,
	// account, and model names. If there exists no model with the
	// specified names, an error satisfying errors.IsNotFound will
	// be returned.
	ModelByName(controllerName, accountName, modelName string) (*ModelDetails, error)
}

// AccountUpdater stores account details.
type AccountUpdater interface {
	// UpdateAccount adds the given account to the account collection.
	//
	// If the account does not already exist, it will be added.
	// Otherwise, it will be overwritten with the new details.
	UpdateAccount(controllerName, accountName string, details AccountDetails) error

	// SetCurrentAccount sets the name of the current account for
	// the specified controller. If there exists no account with
	// the specified names, an error satisfing errors.IsNotFound
	// will be returned.
	SetCurrentAccount(controllerName, accountName string) error
}

// AccountRemover removes accounts.
type AccountRemover interface {
	// RemoveAccount removes the account with the given controller and account
	// names from the accounts collection. If there is no account with the
	// specified names, an errors satisfying errors.IsNotFound will be
	// returned.
	RemoveAccount(controllerName, accountName string) error
}

// AccountGetter gets accounts.
type AccountGetter interface {
	// AllAccounts gets all accounts for the specified controller.
	//
	// If there is no controller with the specified name, or
	// no accounts cached for the controller, an error satisfying
	// errors.IsNotFound will be returned.
	AllAccounts(controllerName string) (map[string]AccountDetails, error)

	// CurrentAccount returns the name of the current account for
	// the specified controller. If there is no current account
	// for the controller, an error satisfying errors.IsNotFound
	// is returned.
	CurrentAccount(controllerName string) (string, error)

	// AccountByName returns the account with the specified controller
	// and account names. If there exists no account with the specified
	// names, an error satisfying errors.IsNotFound will be returned.
	AccountByName(controllerName, accountName string) (*AccountDetails, error)
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

// ClientStore is an amalgamation of AccountStore, ControllerStore, and ModelStore.
type ClientStore interface {
	AccountStore
	ControllerStore
	ModelStore
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

// CredentialStore is an amalgamation of CredentialsUpdater, and CredentialsGetter.
type CredentialStore interface {
	CredentialGetter
	CredentialUpdater
}
