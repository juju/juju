// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/schema"

	"github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/facades/client/application"
	"github.com/juju/juju/controller"
	coreapplication "github.com/juju/juju/core/application"
	corearch "github.com/juju/juju/core/arch"
	corebase "github.com/juju/juju/core/base"
	corecharm "github.com/juju/juju/core/charm"
	coreconfig "github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/unit"
	domainapplication "github.com/juju/juju/domain/application"
	applicationcharm "github.com/juju/juju/domain/application/charm"
	applicationservice "github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/environs/bootstrap"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/charmdownloader"
	"github.com/juju/juju/internal/charm/repository"
	"github.com/juju/juju/internal/configschema"
	"github.com/juju/juju/state"
)

// DeployCharmResult holds the result of deploying a charm.
type DeployCharmInfo struct {
	URL             *charm.URL
	Charm           charm.Charm
	Origin          *corecharm.Origin
	DownloadInfo    *corecharm.DownloadInfo
	ArchivePath     string
	ObjectStoreUUID objectstore.UUID
}

// Validate validates the DeployCharmInfo.
func (d DeployCharmInfo) Validate() error {
	if d.URL == nil {
		return errors.NotValidf("URL is nil")
	}
	if d.Charm == nil {
		return errors.New("Charm is nil")
	}
	if d.Origin == nil {
		return errors.New("Origin is nil")
	}
	if err := d.Origin.Validate(); err != nil {
		return errors.Annotate(err, "Origin")
	}
	return nil
}

// ControllerCharmDeployer is the interface that is used to deploy the
// controller charm.
type ControllerCharmDeployer interface {
	// DeployLocalCharm deploys the controller charm from the local charm
	// store.
	DeployLocalCharm(context.Context, string, corebase.Base) (DeployCharmInfo, error)

	// DeployCharmhubCharm deploys the controller charm from charm hub.
	DeployCharmhubCharm(context.Context, string, corebase.Base) (DeployCharmInfo, error)

	// AddControllerApplication adds the controller application.
	AddControllerApplication(context.Context, DeployCharmInfo, string) (Unit, error)

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
	ModelUUID() string
}

// CharmHubRepoFunc is the function that is used to create a charm repository.
type CharmHubRepoFunc func(repository.CharmHubRepositoryConfig) (corecharm.Repository, error)

// Downloader defines an API for downloading and storing charms.
type Downloader interface {
	// Download looks up the requested charm using the appropriate store, downloads
	// it to a temporary file and passes it to the configured storage API so it can
	// be persisted.
	//
	// The resulting charm is verified to be the right hash. It expected that the
	// origin will always have the correct hash following this call.
	//
	// Returns [ErrInvalidHash] if the hash of the downloaded charm does not match
	// the expected hash.
	Download(ctx context.Context, url *url.URL, hash string) (*charmdownloader.DownloadResult, error)
}

// CharmDownloaderFunc is the function that is used to create a charm
// downloader.
type CharmDownloaderFunc func(HTTPClient, logger.Logger) Downloader

// Application is the interface that is used to get information about an
// application.
type Application interface {
	Name() string
}

// Unit is the interface that is used to get information about a
// controller unit.
type Unit interface {
	// UpdateOperation returns a model operation that will update a unit.
	UpdateOperation(state.UnitUpdateProperties) *state.UpdateUnitOperation
	// AssignToMachineRef assigns this unit to a given machine.
	AssignToMachineRef(state.MachineRef) error
	// UnitTag returns the tag of the unit.
	UnitTag() names.UnitTag
}

// StateBackend is the interface that is used to get information about the
// state.
type StateBackend interface {
	AddApplication(state.AddApplicationArgs, objectstore.ObjectStore) (Application, error)
	Unit(string) (Unit, error)
}

// BaseDeployerConfig holds the configuration for a baseDeployer.
type BaseDeployerConfig struct {
	DataDir              string
	StateBackend         StateBackend
	ApplicationService   ApplicationService
	AgentPasswordService AgentPasswordService
	ModelConfigService   ModelConfigService
	CharmUploader        CharmUploader
	ObjectStore          objectstore.ObjectStore
	Constraints          constraints.Value
	ControllerConfig     controller.Config
	NewCharmHubRepo      CharmHubRepoFunc
	NewCharmDownloader   CharmDownloaderFunc
	CharmhubHTTPClient   HTTPClient
	ControllerCharmName  string
	Channel              charm.Channel
	Logger               logger.Logger
}

// Validate validates the configuration.
func (c BaseDeployerConfig) Validate() error {
	if c.DataDir == "" {
		return errors.NotValidf("DataDir")
	}
	if c.StateBackend == nil {
		return errors.NotValidf("StateBackend")
	}
	if c.ApplicationService == nil {
		return errors.NotValidf("ApplicationService")
	}
	if c.AgentPasswordService == nil {
		return errors.NotValidf("AgentPasswordService")
	}
	if c.ModelConfigService == nil {
		return errors.NotValidf("ModelConfigService")
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
	if c.NewCharmHubRepo == nil {
		return errors.NotValidf("NewCharmHubRepo")
	}
	if c.NewCharmDownloader == nil {
		return errors.NotValidf("NewCharmDownloader")
	}
	if c.CharmhubHTTPClient == nil {
		return errors.NotValidf("CharmhubHTTPClient")
	}
	if c.Logger == nil {
		return errors.NotValidf("Logger")
	}
	return nil
}

type baseDeployer struct {
	dataDir             string
	stateBackend        StateBackend
	applicationService  ApplicationService
	passwordService     AgentPasswordService
	modelConfigService  ModelConfigService
	charmUploader       CharmUploader
	objectStore         objectstore.ObjectStore
	constraints         constraints.Value
	controllerConfig    controller.Config
	newCharmHubRepo     CharmHubRepoFunc
	charmhubHTTPClient  HTTPClient
	charmDownloader     CharmDownloaderFunc
	controllerCharmName string
	channel             charm.Channel
	logger              logger.Logger
}

func makeBaseDeployer(config BaseDeployerConfig) baseDeployer {
	return baseDeployer{
		dataDir:             config.DataDir,
		stateBackend:        config.StateBackend,
		passwordService:     config.AgentPasswordService,
		applicationService:  config.ApplicationService,
		modelConfigService:  config.ModelConfigService,
		charmUploader:       config.CharmUploader,
		objectStore:         config.ObjectStore,
		constraints:         config.Constraints,
		controllerConfig:    config.ControllerConfig,
		newCharmHubRepo:     config.NewCharmHubRepo,
		charmhubHTTPClient:  config.CharmhubHTTPClient,
		charmDownloader:     config.NewCharmDownloader,
		controllerCharmName: config.ControllerCharmName,
		channel:             config.Channel,
		logger:              config.Logger,
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
func (b *baseDeployer) DeployLocalCharm(ctx context.Context, arch string, base corebase.Base) (DeployCharmInfo, error) {
	path := filepath.Join(b.dataDir, "charms", bootstrap.ControllerCharmArchive)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return DeployCharmInfo{}, errors.NotFoundf(path)
	} else if err != nil {
		return DeployCharmInfo{}, errors.Trace(err)
	}

	sha256, sha384, err := b.calculateLocalCharmHashes(path, info.Size())
	if err != nil {
		return DeployCharmInfo{}, errors.Annotatef(err, "calculating hashes for %q", path)
	}

	result, err := b.applicationService.ResolveControllerCharmDownload(ctx, domainapplication.ResolveControllerCharmDownload{
		SHA256: sha256,
		SHA384: sha384,
		Path:   path,
		Size:   info.Size(),
	})
	if err != nil {
		return DeployCharmInfo{}, errors.Annotatef(err, "resolving controller charm download")
	}

	b.logger.Debugf(context.TODO(), "Successfully deployed local Juju controller charm")

	// The revision of the charm will always be zero during bootstrap, so
	// there is no need to have additional logic to determine the revision.
	// Also there is no architecture, because that prevents the URL from
	// being parsed.

	origin := corecharm.Origin{
		Source:   corecharm.Local,
		Type:     "charm",
		Hash:     sha256,
		Revision: ptr(0),
		Platform: corecharm.Platform{
			Architecture: arch,
			OS:           base.OS,
			Channel:      base.Channel.String(),
		},
	}

	return DeployCharmInfo{
		URL: &charm.URL{
			Schema:   charm.Local.String(),
			Name:     bootstrap.ControllerCharmName,
			Revision: 0,
		},
		Charm:           result.Charm,
		Origin:          &origin,
		ArchivePath:     result.ArchivePath,
		ObjectStoreUUID: result.ObjectStoreUUID,
	}, nil
}

// DeployCharmhubCharm deploys the controller charm from charm hub.
func (b *baseDeployer) DeployCharmhubCharm(ctx context.Context, arch string, base corebase.Base) (DeployCharmInfo, error) {
	modelCfg, err := b.modelConfigService.ModelConfig(ctx)
	if err != nil {
		return DeployCharmInfo{}, errors.Trace(err)
	}
	charmhubURL, _ := modelCfg.CharmHubURL()

	charmRepo, err := b.newCharmHubRepo(repository.CharmHubRepositoryConfig{
		Logger:             b.logger,
		CharmhubURL:        charmhubURL,
		CharmhubHTTPClient: b.charmhubHTTPClient,
	})
	if err != nil {
		return DeployCharmInfo{}, errors.Trace(err)
	}

	var curl *charm.URL
	if b.controllerCharmName == "" {
		curl = charm.MustParseURL(controllerCharmURL)
	} else {
		curl = charm.MustParseURL(b.controllerCharmName)
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
	// deployed, we know the exact platform to ask for, no need to review the
	// supported base.
	//
	// We prefer the latest LTS bases, if the current base is not one,
	// charmRepo.ResolveWithPreferredChannel, will return an origin with the
	// latest LTS based on data provided by charmhub in the revision-not-found
	// error response.
	//
	// The controller charm doesn't have any base specific code.
	resolved, err := charmRepo.ResolveWithPreferredChannel(ctx, curl.Name, origin)
	if err != nil {
		return DeployCharmInfo{}, errors.Annotatef(err, "resolving %q", controllerCharmURL)
	}

	downloadInfo := resolved.EssentialMetadata.DownloadInfo

	downloadURL, err := url.Parse(downloadInfo.DownloadURL)
	if err != nil {
		return DeployCharmInfo{}, errors.Annotatef(err, "parsing download URL %q", downloadInfo.DownloadURL)
	}

	charmDownloader := b.charmDownloader(b.charmhubHTTPClient, b.logger)
	downloadResult, err := charmDownloader.Download(ctx, downloadURL, resolved.Origin.Hash)
	if err != nil {
		return DeployCharmInfo{}, errors.Annotatef(err, "downloading %q", downloadURL)
	}

	// We can pass the computed SHA384 because we've ensured that the download
	// SHA256 was correct.

	result, err := b.applicationService.ResolveControllerCharmDownload(ctx, domainapplication.ResolveControllerCharmDownload{
		SHA256: resolved.Origin.Hash,
		SHA384: downloadResult.SHA384,
		Path:   downloadResult.Path,
		Size:   downloadResult.Size,
	})
	if err != nil {
		return DeployCharmInfo{}, errors.Annotatef(err, "resolving controller charm download")
	}

	if resolved.Origin.Revision == nil {
		return DeployCharmInfo{}, errors.Errorf("resolved charm %q has no revision", resolved.URL)
	}

	b.logger.Debugf(context.TODO(), "Successfully deployed charmhub Juju controller charm")

	curl = curl.
		WithRevision(*resolved.Origin.Revision).
		WithArchitecture(resolved.Origin.Platform.Architecture)

	return DeployCharmInfo{
		URL:             curl,
		Charm:           result.Charm,
		Origin:          &resolved.Origin,
		DownloadInfo:    &downloadInfo,
		ArchivePath:     result.ArchivePath,
		ObjectStoreUUID: result.ObjectStoreUUID,
	}, nil
}

// AddControllerApplication adds the controller application.
func (b *baseDeployer) AddControllerApplication(ctx context.Context, info DeployCharmInfo, controllerAddress string) (Unit, error) {
	if err := info.Validate(); err != nil {
		return nil, errors.Trace(err)
	}

	origin := *info.Origin

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

	// Remove this horrible hack once we've removed all of the .Charm calls
	// in the state package. This is just to service the current add
	// application code base.

	stateOrigin.Hash = ""
	stateOrigin.ID = ""

	app, err := b.stateBackend.AddApplication(state.AddApplicationArgs{
		Name:              bootstrap.ControllerApplicationName,
		Charm:             info.Charm,
		CharmURL:          info.URL.String(),
		CharmOrigin:       stateOrigin,
		CharmConfig:       cfg,
		Constraints:       b.constraints,
		ApplicationConfig: appCfg,
		NumUnits:          1,
	}, b.objectStore)
	if err != nil {
		return nil, errors.Annotatef(err, "adding controller application")
	}
	unitName, err := unit.NewNameFromParts(bootstrap.ControllerApplicationName, 0)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// DownloadInfo is not required for local charms, so we only set it if
	// it's not nil.
	if info.URL.Schema == charm.Local.String() && info.DownloadInfo != nil {
		return nil, errors.New("download info should not be set for local charms")
	}

	var downloadInfo *applicationcharm.DownloadInfo
	if info.DownloadInfo != nil {
		downloadInfo = &applicationcharm.DownloadInfo{
			Provenance:         applicationcharm.ProvenanceBootstrap,
			CharmhubIdentifier: info.DownloadInfo.CharmhubIdentifier,
			DownloadURL:        info.DownloadInfo.DownloadURL,
			DownloadSize:       info.DownloadInfo.DownloadSize,
		}
	}

	_, err = b.applicationService.CreateApplication(ctx,
		bootstrap.ControllerApplicationName,
		info.Charm, origin,
		applicationservice.AddApplicationArgs{
			ReferenceName:        bootstrap.ControllerCharmName,
			CharmStoragePath:     info.ArchivePath,
			CharmObjectStoreUUID: info.ObjectStoreUUID,
			DownloadInfo:         downloadInfo,
		},
		applicationservice.AddUnitArg{UnitName: unitName},
	)
	if err != nil {
		return nil, errors.Annotatef(err, "creating controller application")
	}
	return b.stateBackend.Unit(app.Name() + "/0")
}

func (b *baseDeployer) calculateLocalCharmHashes(path string, expectedSize int64) (string, string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", "", errors.Annotatef(err, "opening %q", path)
	}

	hasher256 := sha256.New()
	hasher384 := sha512.New384()

	if size, err := io.Copy(io.MultiWriter(hasher256, hasher384), file); err != nil {
		return "", "", errors.Annotatef(err, "hashing %q", path)
	} else if size != expectedSize {
		return "", "", errors.Errorf("expected %d bytes, got %d", expectedSize, size)
	}

	sha256 := hex.EncodeToString(hasher256.Sum(nil))
	sha384 := hex.EncodeToString(hasher384.Sum(nil))
	return sha256, sha384, nil
}

// ConfigSchema is used to force the trust config option to be true for all
// controllers.
var configSchema = configschema.Fields{
	coreapplication.TrustConfigOptionName: {
		Type: configschema.Tbool,
	},
}

func ptr[T any](v T) *T {
	return &v
}
