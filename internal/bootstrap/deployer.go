// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"net/http"
	"os"
	"path/filepath"

	"github.com/juju/charm/v11"
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
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/environs/bootstrap"
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
	AddControllerApplication(context.Context, string, *corecharm.Origin, string) (ControllerUnit, error)

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
	CompleteProcess(context.Context, ControllerUnit) error
}

// ControllerUnit is the interface that is used to get information about a
// controller unit.
type ControllerUnit interface {
	// UpdateOperation returns a model operation that will update a unit.
	UpdateOperation(state.UnitUpdateProperties) *state.UpdateUnitOperation
	// AssignToMachine assigns this unit to a given machine.
	AssignToMachine(*state.Machine) error
	// UnitTag returns the tag of the unit.
	UnitTag() names.UnitTag
	// SetPassword sets the password for the unit.
	SetPassword(string) error
}

// Machine is the interface that is used to get information about a machine.
type Machine interface {
	// PublicAddress returns a public address for the machine. If no address is
	// available it returns an error that satisfies network.IsNoAddressError().
	PublicAddress() (network.SpaceAddress, error)

	// Base returns the underlying base of the machine.
	Base() state.Base
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

// CharmRepoFunc is the function that is used to create a charm repository.
type CharmRepoFunc func(services.CharmRepoFactoryConfig) (corecharm.Repository, error)

// CharmDownloaderFunc is the function that is used to create a charm
// downloader.
type CharmDownloaderFunc func(services.CharmDownloaderConfig) (interfaces.Downloader, error)

// BaseDeployerConfig holds the configuration for a baseDeployer.
type BaseDeployerConfig struct {
	DataDir            string
	State              *state.State
	ObjectStore        objectstore.ObjectStore
	Constraints        constraints.Value
	ControllerConfig   controller.Config
	NewCharmRepo       CharmRepoFunc
	NewCharmDownloader CharmDownloaderFunc
	CharmhubHTTPClient HTTPClient
	CharmhubURL        string
	Channel            charm.Channel
	LoggerFactory      LoggerFactory
}

// Validate validates the configuration.
func (c BaseDeployerConfig) Validate() error {
	if c.DataDir == "" {
		return errors.NotValidf("DataDir")
	}
	if c.State == nil {
		return errors.NotValidf("State")
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
	dataDir            string
	state              *state.State
	objectStore        objectstore.ObjectStore
	constraints        constraints.Value
	controllerConfig   controller.Config
	newCharmRepo       CharmRepoFunc
	newCharmDownloader CharmDownloaderFunc
	charmhubHTTPClient HTTPClient
	charmhubURL        string
	channel            charm.Channel
	loggerFactory      LoggerFactory
	logger             Logger
}

func makeBaseDeployer(config BaseDeployerConfig) baseDeployer {
	return baseDeployer{
		dataDir:            config.DataDir,
		state:              config.State,
		objectStore:        config.ObjectStore,
		constraints:        config.Constraints,
		controllerConfig:   config.ControllerConfig,
		newCharmRepo:       config.NewCharmRepo,
		newCharmDownloader: config.NewCharmDownloader,
		charmhubHTTPClient: config.CharmhubHTTPClient,
		charmhubURL:        config.CharmhubURL,
		channel:            config.Channel,
		loggerFactory:      config.LoggerFactory,
		logger:             config.LoggerFactory.Child("deployer"),
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
func (b *baseDeployer) DeployLocalCharm(ctx context.Context, arch string, base corebase.Base) (*charm.URL, *corecharm.Origin, error) {
	controllerCharmPath := filepath.Join(b.dataDir, "charms", bootstrap.ControllerCharmArchive)
	_, err := os.Stat(controllerCharmPath)
	if os.IsNotExist(err) {
		return nil, nil, errors.NotFoundf(controllerCharmPath)
	}
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	curl, err := addLocalControllerCharm(ctx, b.objectStore, b.state, base, controllerCharmPath)
	if err != nil {
		return nil, nil, errors.Annotatef(err, "cannot store controller charm at %q", controllerCharmPath)
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
	return curl, &origin, nil
}

// DeployCharmhubCharm deploys the controller charm from charm hub.
func (b *baseDeployer) DeployCharmhubCharm(ctx context.Context, arch string, base corebase.Base) (string, *corecharm.Origin, error) {
	model, err := b.state.Model()
	if err != nil {
		return "", nil, err
	}

	stateBackend := &stateShim{State: b.state}
	charmRepo, err := b.newCharmRepo(services.CharmRepoFactoryConfig{
		LoggerFactory:      b.loggerFactory,
		CharmhubHTTPClient: b.charmhubHTTPClient,
		StateBackend:       stateBackend,
		ModelBackend:       model,
	})
	if err != nil {
		return "", nil, err
	}

	var curl *charm.URL
	if b.charmhubURL == "" {
		curl = charm.MustParseURL(controllerCharmURL)
	} else {
		curl = charm.MustParseURL(b.charmhubURL)
	}
	if err != nil {
		return "", nil, err
	}
	origin := corecharm.Origin{
		Source:  corecharm.CharmHub,
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
		StateBackend:       stateBackend,
		ModelBackend:       model,
	})
	if err != nil {
		return "", nil, err
	}
	resOrigin, err := charmDownloader.DownloadAndStore(ctx, curl, origin, false)
	if err != nil {
		return "", nil, err
	}

	b.logger.Debugf("Successfully deployed charmhub Juju controller charm")

	return curl.String(), &resOrigin, nil
}

// AddControllerApplication adds the controller application.
func (b *baseDeployer) AddControllerApplication(ctx context.Context, curl string, origin corecharm.Origin, controllerAddress string) (ControllerUnit, error) {
	ch, err := b.state.Charm(curl)
	if err != nil {
		return nil, errors.Trace(err)
	}
	cfg := charm.Settings{
		"is-juju": true,
	}
	cfg["identity-provider-url"] = b.controllerConfig.IdentityURL()
	addr := b.controllerConfig.PublicDNSAddress()
	if addr == "" {
		addr = controllerAddress
	}
	if addr != "" {
		cfg["controller-url"] = api.ControllerAPIURL(addr, b.controllerConfig.APIPort())
	}

	appCfg, err := config.NewConfig(nil, configSchema, schema.Defaults{
		coreapplication.TrustConfigOptionName: true,
	})
	if err != nil {
		return nil, errors.Trace(err)
	}

	stateOrigin, err := application.StateCharmOrigin(origin)
	if err != nil {
		return nil, errors.Trace(err)
	}

	app, err := b.state.AddApplication(state.AddApplicationArgs{
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
	return b.state.Unit(app.Name() + "/0")
}

// addLocalControllerCharm adds the specified local charm to the controller.
func addLocalControllerCharm(ctx context.Context, objectStore services.Storage, st *state.State, base corebase.Base, charmFileName string) (*charm.URL, error) {
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
	curl, err = st.PrepareLocalCharmUpload(curl.String())
	if err != nil {
		return nil, errors.Trace(err)
	}

	// Now we need to repackage it with the reserved URL, upload it to
	// provider storage and update the state.
	err = apiserver.RepackageAndUploadCharm(ctx, objectStore, st, archive, curl)
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

// stateShim allows us to use a real state instance with the charm services logic.
type stateShim struct {
	*state.State
}

func (st *stateShim) PrepareCharmUpload(curl string) (services.UploadedCharm, error) {
	return st.State.PrepareCharmUpload(curl)
}

func (st *stateShim) UpdateUploadedCharm(info state.CharmInfo) (services.UploadedCharm, error) {
	return st.State.UpdateUploadedCharm(info)
}
