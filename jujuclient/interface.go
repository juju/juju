// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"net/http"

	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/proxy"
)

// ControllerDetails holds the details needed to connect to a controller.
type ControllerDetails struct {
	// ControllerUUID is the unique ID for the controller.
	ControllerUUID string `yaml:"uuid"`

	// APIEndpoints holds a list of API addresses. It may not be
	// current, and it will be empty if the environment has not been
	// bootstrapped.
	APIEndpoints []string `yaml:"api-endpoints,flow"`

	// DNSCache holds a map of hostname to IP addresses, holding
	// a cache of the last time the API endpoints were looked up.
	// The information held here is strictly optional so that we
	// can avoid slow DNS queries in the usual case that the controller's
	// IP addresses haven't changed since the last time we connected.
	DNSCache map[string][]string `yaml:"dns-cache,omitempty,flow"`

	// PublicDNSName holds the public host name associated with the controller.
	// If this is non-empty, it indicates that the controller will use an
	// officially signed certificate when connecting with this host name.
	PublicDNSName string `yaml:"public-hostname,omitempty"`

	// CACert is a security certificate for this controller.
	CACert string `yaml:"ca-cert"`

	// Cloud is the name of the cloud that this controller runs in.
	Cloud string `yaml:"cloud"`

	// CloudRegion is the name of the cloud region that this controller
	// runs in. This will be empty for clouds without regions.
	CloudRegion string `yaml:"region,omitempty"`

	// CloudType is the type of the cloud that this controller runs in.
	CloudType string `yaml:"type,omitempty"`

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

	// MachineCount is the number of machines in all models to
	// which a user has access. It is cached here so under normal
	// usage list-controllers does not need to hit the server.
	MachineCount *int `yaml:"machine-count,omitempty"`

	Proxy *ProxyConfWrapper `yaml:"proxy-config,omitempty"`
}

type ProxyConfWrapper struct {
	Proxier proxy.Proxier
}

func (p *ProxyConfWrapper) MarshalYAML() (interface{}, error) {
	return map[string]interface{}{
		"type":   p.Proxier.Type(),
		"config": p.Proxier,
	}, nil
}

func (p *ProxyConfWrapper) UnmarshalYAML(unmarshal func(interface{}) error) error {
	factory, err := proxy.NewDefaultFactory()
	if err != nil {
		return errors.Annotate(err, "building proxy factory for config")
	}

	proxyConf := struct {
		Type   string    `yaml:"type"`
		Config yaml.Node `yaml:config"`
	}{}

	err = unmarshal(&proxyConf)
	if err != nil {
		return errors.Annotate(err, "unmarshalling raw proxy config")
	}

	maker, err := factory.MakerForTypeKey(proxyConf.Type)
	if err != nil {
		return errors.Trace(err)
	}

	if err = proxyConf.Config.Decode(maker.Config()); err != nil {
		return errors.Annotatef(err, "deconding config for proxy of type %s", proxyConf.Type)
	}

	p.Proxier, err = maker.Make()
	if err != nil {
		return errors.Annotatef(err, "making proxier for type %s", proxyConf.Type)
	}

	return nil
}

// ModelDetails holds details of a model.
type ModelDetails struct {
	// ModelUUID is the unique ID for the model.
	ModelUUID string `yaml:"uuid"`

	// ModelType is the type of model.
	ModelType model.ModelType `yaml:"type"`

	// Active branch is the current working branch for the model.
	ActiveBranch string `yaml:"branch"`
}

// AccountDetails holds details of an account.
type AccountDetails struct {
	// User is the username for the account.
	User string `yaml:"user"`

	// Password is the password for the account.
	Password string `yaml:"password,omitempty"`

	// LastKnownAccess is the last known access level for the account.
	LastKnownAccess string `yaml:"last-known-access,omitempty"`

	// Macaroons, if set, are used for the account login.
	// They are only set when using the MemStore implementation,
	// and are used by embedded commands. The are not written to disk.
	Macaroons []macaroon.Slice `yaml:"-"`
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

	// CloudCACertificates contains the CACertificates necessary to
	// communicate with the cloud infrastructure.
	CloudCACertificates []string `yaml:"ca-certificates,omitempty"`

	// SkipTLSVerify is true if the client should be asked not to
	// validate certificates. It is not recommended for production clouds.
	// It is secure (false) by default.
	SkipTLSVerify bool `yaml:"skip-tls-verify,omitempty"`
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

	// ControllerByAPIEndpoints returns the details and name of the
	// controller where the set of API endpoints contains any of the
	// provided endpoints. If no controller can be located, an error
	// satisfying errors.IsNotFound will be returned.
	ControllerByAPIEndpoints(endpoints ...string) (*ControllerDetails, string, error)

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

	// SetModels updates the list of currently stored controller
	// models in model store - models will be added, updated or removed from the
	// store based on the supplied models collection.
	SetModels(controllerName string, models map[string]ModelDetails) error

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

// CookieJar is the interface implemented by cookie jars.
type CookieJar interface {
	http.CookieJar

	// RemoveAll removes all the cookies (note: this doesn't
	// save the cookie file).
	RemoveAll()

	// Save saves the cookies.
	Save() error
}

// CookieStore allows the retrieval of cookie jars for storage
// of per-controller authorization information.
type CookieStore interface {
	CookieJar(controllerName string) (CookieJar, error)
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
	CookieStore
}
