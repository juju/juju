// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/charm/v12"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/schema"
	"gopkg.in/juju/environschema.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/apiserver/facades/client/charms/interfaces"
	"github.com/juju/juju/apiserver/facades/client/charms/services"
	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// LoggerFactory is the interface that is used to create loggers.
type LoggerFactory interface {
	services.LoggerFactory
}

// ControllerCharmDeployer is the interface that is used to deploy the
// controller charm.
type ControllerCharmDeployer interface {
	// DeployLocalCharm deploys the controller charm from the local charm
	// store.
	DeployLocalCharm(context.Context, string, corebase.Base) (string, *corecharm.Origin, error)

	// DeployCharmhubCharm deploys the controller charm from charm hub.
	DeployCharmhubCharm(context.Context, string, corebase.Base) (string, *corecharm.Origin, error)

	// AddControllerApplication adds the controller application.
	AddControllerApplication(context.Context, string, corecharm.Origin, string) (Unit, error)

	// ControllerAddress returns the address of the controller that should be
	// used.
	ControllerAddress(context.Context) (string, error)

	// ControllerCharmBase returns the base used for deploying the controller
	// charm.
	ControllerCharmBase() (corebase.Base, error)

	// ControllerCharmArch returns the architecture used for deploying the
	// controller charm.
	ControllerCharmArch() string

	// CompleteProcess is called when the bootstrap process is complete.
	CompleteProcess(context.Context, Unit) error
}

// Machine is the interface that is used to get information about a machine.
type Machine interface {
	DocID() string
	Id() string
	MachineTag() names.MachineTag
	Life() state.Life
	Clean() bool
	ContainerType() instance.ContainerType
	Base() state.Base
	Jobs() []state.MachineJob
	AddPrincipal(string)
	FileSystems() []string
	PublicAddress() (network.SpaceAddress, error)
}

// MachineGetter is the interface that is used to get information about a
// machine.
type MachineGetter interface {
	Machine(string) (Machine, error)
}

// HTTPClient is the interface that is used to make HTTP requests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// CharmUploader is an interface that is used to update the charm in
// state and upload it to the object store.
type CharmUploader interface {
	PrepareLocalCharmUpload(url string) (chosenURL *charm.URL, err error)
	UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error)
	PrepareCharmUpload(curl string) (services.UploadedCharm, error)
	ModelUUID() string
}

// CharmRepoFunc is the function that is used to create a charm repository.
type CharmRepoFunc func(services.CharmRepoFactoryConfig) (corecharm.Repository, error)

// CharmDownloaderFunc is the function that is used to create a charm
// downloader.
type CharmDownloaderFunc func(services.CharmDownloaderConfig) (interfaces.Downloader, error)

// Application is the interface that is used to get information about an
// application.
type Application interface {
	Name() string
}

// Charm is the interface that is used to get information about a charm.
type Charm interface {
	Meta() *charm.Meta
	Manifest() *charm.Manifest
	URL() string
}

// Model is the interface that is used to get information about a model.
type Model interface {
	Config() (*config.Config, error)
}

// Unit is the interface that is used to get information about a
// controller unit.
type Unit interface {
	// UpdateOperation returns a model operation that will update a unit.
	UpdateOperation(state.UnitUpdateProperties) *state.UpdateUnitOperation
	// AssignToMachine assigns this unit to a given machine.
	AssignToMachineRef(state.MachineRef) error
	// UnitTag returns the tag of the unit.
	UnitTag() names.UnitTag
	// SetPassword sets the password for the unit.
	SetPassword(string) error
}

// StateBackend is the interface that is used to get information about the
// state.
type StateBackend interface {
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (Application, error)
	Charm(string) (Charm, error)
	Model() (Model, error)
	Unit(string) (Unit, error)
}

// BaseDeployerConfig holds the configuration for a baseDeployer.
type BaseDeployerConfig struct {
	DataDir             string
	StateBackend        StateBackend
	CharmUploader       CharmUploader
	ObjectStore         objectstore.ObjectStore
	Constraints         constraints.Value
	ControllerConfig    controller.Config
	NewCharmRepo        CharmRepoFunc
	NewCharmDownloader  CharmDownloaderFunc
	CharmhubHTTPClient  HTTPClient
	ControllerCharmName string
	Channel             charm.Channel
	LoggerFactory       LoggerFactory
}

// Validate validates the configuration.
func (c BaseDeployerConfig) Validate() error {
	if c.DataDir == "" {
		return errors.NotValidf("DataDir")
	}
	if c.StateBackend == nil {
		return errors.NotValidf("StateBackend")
	}
	if c.CharmUploader == nil {
		return errors.NotValidf("CharmUploader")
	}
	if c.ObjectStore == nil {
		return errors.NotValidf("ObjectStore")
	}
	if c.ControllerConfig == nil {
		return errors.NotValidf("ControllerConfig")
	}
	if c.NewCharmRepo == nil {
		return errors.NotValidf("NewCharmRepo")
	}
	if c.NewCharmDownloader == nil {
		return errors.NotValidf("NewCharmDownloader")
	}
	if c.CharmhubHTTPClient == nil {
		return errors.NotValidf("CharmhubHTTPClient")
	}
	if c.LoggerFactory == nil {
		return errors.NotValidf("LoggerFactory")
	}
	return nil
}

type baseDeployer struct {
	dataDir             string
	stateBackend        StateBackend
	charmUploader       CharmUploader
	objectStore         objectstore.ObjectStore
	constraints         constraints.Value
	controllerConfig    controller.Config
	newCharmRepo        CharmRepoFunc
	newCharmDownloader  CharmDownloaderFunc
	charmhubHTTPClient  HTTPClient
	controllerCharmName string
	channel             charm.Channel
	loggerFactory       LoggerFactory
	logger              Logger
}

func makeBaseDeployer(config BaseDeployerConfig) baseDeployer {
	return baseDeployer{
		dataDir:             config.DataDir,
		stateBackend:        config.StateBackend,
		charmUploader:       config.CharmUploader,
		objectStore:         config.ObjectStore,
		constraints:         config.Constraints,
		controllerConfig:    config.ControllerConfig,
		newCharmRepo:        config.NewCharmRepo,
		newCharmDownloader:  config.NewCharmDownloader,
		charmhubHTTPClient:  config.CharmhubHTTPClient,
		controllerCharmName: config.ControllerCharmName,
		channel:             config.Channel,
		loggerFactory:       config.LoggerFactory,
		logger:              config.LoggerFactory.Child("deployer"),
	}
}

// ControllerCharmArch returns the architecture used for deploying the
// controller charm.
func (b *baseDeployer) ControllerCharmArch() string {
	arch := corearch.DefaultArchitecture
	if b.constraints.HasArch() {
		arch = *b.constraints.Arch
	}
	return arch
}

// DeployLocalCharm deploys the controller charm from the local charm
// store.
func (b *baseDeployer) DeployLocalCharm(ctx context.Context, arch string, base corebase.Base) (string, *corecharm.Origin, error) {
	controllerCharmPath := filepath.Join(b.dataDir, "charms", bootstrap.ControllerCharmArchive)
	_, err := os.Stat(controllerCharmPath)
	if os.IsNotExist(err) {
		return "", nil, errors.NotFoundf(controllerCharmPath)
	}
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	curl, err := addLocalControllerCharm(ctx, b.objectStore, b.charmUploader, base, controllerCharmPath)
	if err != nil {
		return "", nil, errors.Annotatef(err, "cannot store controller charm at %q", controllerCharmPath)
	}
	b.logger.Debugf("Successfully deployed local Juju controller charm")
	origin := corecharm.Origin{
		Source: corecharm.Local,
		Type:   "charm",
		Platform: corecharm.Platform{
			Architecture: arch,
			OS:           base.OS,
			Channel:      base.Channel.String(),
		},
	}
	return curl.String(), &origin, nil
}

// DeployCharmhubCharm deploys the controller charm from charm hub.
func (b *baseDeployer) DeployCharmhubCharm(ctx context.Context, arch string, base corebase.Base) (string, *corecharm.Origin, error) {
	model, err := b.stateBackend.Model()
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	charmRepo, err := b.newCharmRepo(services.CharmRepoFactoryConfig{
		LoggerFactory:      b.loggerFactory,
		CharmhubHTTPClient: b.charmhubHTTPClient,
		StateBackend:       b.charmUploader,
		ModelBackend:       model,
	})
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	var curl *charm.URL
	if b.controllerCharmName == "" {
		curl = charm.MustParseURL(controllerCharmURL)
	} else {
		curl = charm.MustParseURL(b.controllerCharmName)
	}
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	origin := corecharm.Origin{
		Source:  corecharm.CharmHub,
		Type:    "charm",
		Channel: &b.channel,
		Platform: corecharm.Platform{
			Architecture: arch,
			OS:           base.OS,
			Channel:      base.Channel.Track,
		},
	}

	// Since we're running on the machine to which the controller charm will be
	// deployed, we know the exact platform to ask for, not need to review the
	// supported series.
	//
	// We prefer the latest LTS series, if the current series is not one,
	// charmRepo.ResolveWithPreferredChannel, will return an origin with the
	// latest LTS based on data provided by charmhub in the revision-not-found
	// error response.
	//
	// The controller charm doesn't have any series specific code.
	curl, origin, _, err = charmRepo.ResolveWithPreferredChannel(ctx, curl.Name, origin)
	if err != nil {
		return "", nil, errors.Annotatef(err, "resolving %q", controllerCharmURL)
	}

	charmDownloader, err := b.newCharmDownloader(services.CharmDownloaderConfig{
		LoggerFactory:      b.loggerFactory,
		CharmhubHTTPClient: b.charmhubHTTPClient,
		ObjectStore:        b.objectStore,
		StateBackend:       b.charmUploader,
		ModelBackend:       model,
	})
	if err != nil {
		return "", nil, errors.Trace(err)
	}
	resOrigin, err := charmDownloader.DownloadAndStore(ctx, curl, origin, false)
	if err != nil {
		return "", nil, errors.Trace(err)
	}

	b.logger.Debugf("Successfully deployed charmhub Juju controller charm")

	return curl.String(), &resOrigin, nil
}

// AddControllerApplication adds the controller application.
func (b *baseDeployer) AddControllerApplication(ctx context.Context, curl string, origin corecharm.Origin, controllerAddress string) (Unit, error) {
	ch, err := b.stateBackend.Charm(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg := charm.Settings{
		"is-juju": true,
	}
	cfg["identity-provider-url"] = b.controllerConfig.IdentityURL()

	// Attempt to set the controller URL on to the controller charm config.
	addr := b.controllerConfig.PublicDNSAddress()
	if addr == "" {
		addr = controllerAddress
	}
	if addr != "" {
		cfg["controller-url"] = api.ControllerAPIURL(addr, b.controllerConfig.APIPort())
	}

	appCfg, err := coreconfig.NewConfig(nil, configSchema, schema.Defaults{
		coreapplication.TrustConfigOptionName: true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	stateOrigin, err := application.StateCharmOrigin(origin)
	if err != nil {
		return nil, errors.Trace(err)
	}

	app, err := b.stateBackend.AddApplication(state.AddApplicationArgs{
		Name:              bootstrap.ControllerApplicationName,
		Charm:             ch,
		CharmOrigin:       stateOrigin,
		CharmConfig:       cfg,
		Constraints:       b.constraints,
		ApplicationConfig: appCfg,
		NumUnits:          1,
	}, b.objectStore)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return b.stateBackend.Unit(app.Name() + "/0")
}

// addLocalControllerCharm adds the specified local charm to the controller.
func addLocalControllerCharm(ctx context.Context, objectStore services.Storage, uploader CharmUploader, base corebase.Base, charmFileName string) (*charm.URL, error) {
	archive, err := charm.ReadCharmArchive(charmFileName)
	if err != nil {
		return nil, errors.Errorf("invalid charm archive: %v", err)
	}

	name := archive.Meta().Name
	if name != bootstrap.ControllerCharmName {
		return nil, errors.Errorf("unexpected controller charm name %q", name)
	}

	series, err := corebase.GetSeriesFromBase(base)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Reserve a charm URL for it in state.
	curl := &charm.URL{
		Schema:   charm.Local.String(),
		Name:     name,
		Revision: archive.Revision(),
		Series:   series,
	}
	curl, err = uploader.PrepareLocalCharmUpload(curl.String())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = apiserver.RepackageAndUploadCharm(ctx, objectStore, uploader, archive, curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return curl, nil
}

// ConfigSchema is used to force the trust config option to be true for all
// controllers.
var configSchema = environschema.Fields{
	coreapplication.TrustConfigOptionName: {
		Type: environschema.Tbool,
	},
}
