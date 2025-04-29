// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/juju/clock"
	"github.com/juju/collections/set"
	"github.com/juju/description/v9"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/logger"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/semversion"
	corestorage "github.com/juju/juju/core/storage"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(
	coordinator Coordinator,
	registry corestorage.ModelStorageRegistryGetter,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&importOperation{
		registry: registry,
		clock:    clock,
		logger:   logger,
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	service ImportService

	registry corestorage.ModelStorageRegistryGetter
	clock    clock.Clock
	logger   logger.Logger
}

// ImportService defines the application service used to import applications
// from another controller model to this controller.
type ImportService interface {
	// ImportApplication registers the existence of an application in the model.
	ImportApplication(context.Context, string, service.ImportApplicationArgs) error

	// RemoveImportedApplication removes an application that was imported. The
	// application might be in an incomplete state, so it's important to remove
	// as much of the application as possible, even on failure.
	RemoveImportedApplication(context.Context, string) error

	// GetSpaceUUIDByName returns the UUID of the space with the given name.
	//
	// It returns an error satisfying [networkerrors.SpaceNotFound] if the provided
	// space name doesn't exist.
	GetSpaceUUIDByName(ctx context.Context, name string) (network.Id, error)
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import applications"
}

// Setup creates the service that is used to import applications.
func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewMigrationService(
		state.NewState(scope.ModelDB(), i.clock, i.logger),
		i.registry,
		i.clock,
		i.logger,
	)
	return nil
}

// Execute the import, adding the application to the model. This also includes
// the charm and any units that are associated with the application.
func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	// Ensure we import principal applications first, so that
	// subordinate units can refer to the principal ones.
	var principals, subordinates []description.Application
	for _, app := range model.Applications() {
		if app.Subordinate() {
			subordinates = append(subordinates, app)
		} else {
			principals = append(principals, app)
		}
	}

	modelType, err := coremodel.ParseModelType(model.Type())
	if err != nil {
		return errors.Errorf("parsing model type %q: %w", model.Type(), err)
	}

	for _, app := range append(principals, subordinates...) {
		unitArgs := make([]service.ImportUnitArg, 0, len(app.Units()))
		for _, unit := range app.Units() {
			var (
				unitArg service.ImportUnitArg
				err     error
			)
			switch modelType {
			case coremodel.CAAS:
				unitArg, err = i.importCAASUnit(ctx, unit)
			case coremodel.IAAS:
				unitArg, err = i.importIAASUnit(ctx, unit)
			default:
				return errors.Errorf("unknown model type %q", modelType)
			}
			if err != nil {
				return errors.Errorf("importing unit %q: %w", unit.Name(), err)
			}
			unitArgs = append(unitArgs, unitArg)
		}

		chURL, err := internalcharm.ParseURL(app.CharmURL())
		if err != nil {
			return errors.Errorf("parsing charm URL %q: %w", app.CharmURL(), err)
		}

		charm, err := i.importCharm(ctx, charmData{
			Metadata: app.CharmMetadata(),
			Manifest: app.CharmManifest(),
			Actions:  app.CharmActions(),
			Config:   app.CharmConfigs(),
		})
		if err != nil {
			return errors.Errorf("importing model application %q charm: %w", app.Name(), err)
		}

		origin, err := i.importCharmOrigin(app)
		if err != nil {
			return errors.Errorf("parsing charm origin %v: %w", app.CharmOrigin(), err)
		}

		applicationConfig, err := i.importApplicationConfig(app)
		if err != nil {
			return errors.Errorf("importing application config: %w", err)
		}

		applicationSettings, err := i.importApplicationSettings(app)
		if err != nil {
			return errors.Errorf("importing application settings: %w", err)
		}

		scaleState := application.ScaleState{
			Scale: app.DesiredScale(),
		}

		if provisioningState := app.ProvisioningState(); provisioningState != nil {
			scaleState.Scaling = provisioningState.Scaling()
			scaleState.ScaleTarget = provisioningState.ScaleTarget()
		}

		endpointBindings, err := i.importEndpointBindings(app, model.Spaces())
		if err != nil {
			return errors.Errorf("importing endpoint bindings: %w", err)
		}

		exposedEndpoints, err := i.importExposedEndpoints(ctx, app, model.Spaces())
		if err != nil {
			return errors.Errorf("importing exposed endpoints: %w", err)
		}

		err = i.service.ImportApplication(ctx, app.Name(), service.ImportApplicationArgs{
			Charm:                  charm,
			CharmOrigin:            origin,
			Units:                  unitArgs,
			ApplicationConfig:      applicationConfig,
			ApplicationSettings:    applicationSettings,
			ApplicationConstraints: i.importApplicationConstraints(app),
			ScaleState:             scaleState,
			EndpointBindings:       endpointBindings,
			ExposedEndpoints:       exposedEndpoints,

			// ReferenceName is the name of the charm URL, not the application
			// name and not the charm name in the metadata, but the name of
			// the charm from the store if it's a charm from the store.
			ReferenceName: chURL.Name,
		})
		if err != nil {
			return errors.Errorf(
				"import model application %q with %d units: %w",
				app.Name(), len(app.Units()), err,
			)
		}
	}

	return nil
}

// Rollback the import operation. This is required to remove any applications
// that were added during the import operation.
// For instance, if multiple applications are add, each with their own
// transaction, then if one fails, the others should be rolled back.
func (i *importOperation) Rollback(ctx context.Context, model description.Model) error {
	var errs []error
	for _, app := range model.Applications() {
		if err := i.service.RemoveImportedApplication(ctx, app.Name()); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Errorf("rollback failed: %w", errors.Join(errs...))
}

func (i *importOperation) importApplicationConfig(app description.Application) (config.ConfigAttributes, error) {
	// Application config is optional, so if we don't have any data, we can just
	// return an empty config.
	appConfig := app.CharmConfig()
	if len(appConfig) == 0 {
		return nil, nil
	}

	charmConfig := app.CharmConfigs()
	if charmConfig == nil || len(charmConfig.Configs()) == 0 {
		// Potentially, we could relax this error to a warning and just ignore
		// the config, but it's better to be strict and ensure that the charm
		// config is present.
		return nil, errors.Errorf("charm config is empty, but application %q has config", app.Name())
	}

	charmCfg := charmConfig.Configs()

	result := make(config.ConfigAttributes)
	for k, v := range appConfig {
		if _, ok := charmCfg[k]; !ok {
			// This shouldn't happen, and could be a warning like above, but
			// it's better to be strict and ensure that the charm config is
			// present.
			return nil, errors.Errorf("config %q not found in charm config", k)
		}

		result[k] = v
	}
	return result, nil
}

func (i *importOperation) importApplicationSettings(app description.Application) (application.ApplicationSettings, error) {
	// Application settings are optional, so if we don't have any data, we can
	// just return an empty settings.
	appSettings := app.ApplicationConfig()
	if len(appSettings) == 0 {
		return application.ApplicationSettings{}, nil
	} else if len(appSettings) > 1 {
		return application.ApplicationSettings{}, errors.Errorf("application %q has multiple settings, expected only one", app.Name())
	}

	var trust bool

	trustValue := appSettings[coreapplication.TrustConfigOptionName]
	switch t := trustValue.(type) {
	case string:
		var err error
		if trust, err = strconv.ParseBool(t); err != nil {
			return application.ApplicationSettings{}, errors.Errorf("parsing trust value %q: %w", t, err)
		}
	case bool:
		trust = t
	default:
		return application.ApplicationSettings{}, errors.Errorf("trust value %q is not a boolean", trustValue)
	}

	return application.ApplicationSettings{
		Trust: trust,
	}, nil
}

func (i *importOperation) importApplicationConstraints(app description.Application) constraints.Value {
	result := constraints.Value{}

	cons := app.Constraints()
	if cons == nil {
		return result
	}

	if allocate := cons.AllocatePublicIP(); allocate {
		result.AllocatePublicIP = &allocate
	}
	if arch := cons.Architecture(); arch != "" {
		result.Arch = &arch
	}
	if container := instance.ContainerType(cons.Container()); container != "" {
		result.Container = &container
	}
	if cores := cons.CpuCores(); cores != 0 {
		result.CpuCores = &cores
	}
	if power := cons.CpuPower(); power != 0 {
		result.CpuPower = &power
	}
	if inst := cons.InstanceType(); inst != "" {
		result.InstanceType = &inst
	}
	if mem := cons.Memory(); mem != 0 {
		result.Mem = &mem
	}
	if imageID := cons.ImageID(); imageID != "" {
		result.ImageID = &imageID
	}
	if disk := cons.RootDisk(); disk != 0 {
		result.RootDisk = &disk
	}
	if source := cons.RootDiskSource(); source != "" {
		result.RootDiskSource = &source
	}
	if spaces := cons.Spaces(); len(spaces) > 0 {
		result.Spaces = &spaces
	}
	if tags := cons.Tags(); len(tags) > 0 {
		result.Tags = &tags
	}
	if virt := cons.VirtType(); virt != "" {
		result.VirtType = &virt
	}
	if zones := cons.Zones(); len(zones) > 0 {
		result.Zones = &zones
	}

	return result
}

// importCharmOrigin returns the charm origin for an application
//
// Ensure ID, Hash and Channel are dropped from local charm.
// Due to LP:1986547: where the track is missing from the effective channel it implicitly
// resolves to 'latest' if the charm does not have a default channel defined. So if the
// received channel has no track, we can be confident it should be 'latest'
func (i *importOperation) importCharmOrigin(a description.Application) (corecharm.Origin, error) {
	sourceOrigin := a.CharmOrigin()
	if sourceOrigin == nil {
		return corecharm.Origin{}, errors.Errorf("nil charm origin importing application %q", a.Name())
	}
	_, err := internalcharm.ParseURL(a.CharmURL())
	if err != nil {
		return corecharm.Origin{}, errors.Capture(err)
	}

	var channel *internalcharm.Channel
	serialized := sourceOrigin.Channel()
	if serialized != "" && corecharm.CharmHub.Matches(sourceOrigin.Source()) {
		c, err := internalcharm.ParseChannelNormalize(serialized)
		if err != nil {
			return corecharm.Origin{}, errors.Capture(err)
		}
		track := c.Track
		if track == "" {
			track = "latest"
		}
		channel = &internalcharm.Channel{
			Track:  track,
			Risk:   c.Risk,
			Branch: c.Branch,
		}
	}

	p, err := corecharm.ParsePlatformNormalize(sourceOrigin.Platform())
	if err != nil {
		return corecharm.Origin{}, errors.Capture(err)
	}
	platform := corecharm.Platform{
		Architecture: p.Architecture,
		OS:           p.OS,
		Channel:      p.Channel,
	}

	rev := sourceOrigin.Revision()
	// We can hardcode type to charm as we never store bundles in state.
	var origin corecharm.Origin
	if corecharm.Local.Matches(sourceOrigin.Source()) {
		origin = corecharm.Origin{
			Source:   corecharm.Local,
			Type:     "charm",
			Revision: &rev,
			Platform: platform,
		}
	} else if corecharm.CharmHub.Matches(sourceOrigin.Source()) {
		origin = corecharm.Origin{
			Source:   corecharm.CharmHub,
			Type:     "charm",
			Revision: &rev,
			Platform: platform,
			ID:       sourceOrigin.ID(),
			Hash:     sourceOrigin.Hash(),
			Channel:  channel,
		}
	} else {
		return corecharm.Origin{}, errors.Errorf("unrecognised charm origin %q", sourceOrigin.Source())
	}
	return origin, nil
}

func (i *importOperation) makeAddress(addr description.Address) (*network.SpaceAddress, *network.Origin) {
	if addr == nil {
		return nil, nil
	}

	result := &network.SpaceAddress{
		MachineAddress: network.MachineAddress{
			Value: addr.Value(),
			Type:  network.AddressType(addr.Type()),
			Scope: network.Scope(addr.Scope()),
		},
		SpaceID: addr.SpaceID(),
	}

	// Addresses are placed in the default space if no space ID is set.
	if result.SpaceID == "" {
		result.SpaceID = network.AlphaSpaceId
	}

	return result, ptr(network.Origin(addr.Origin()))
}

type charmData struct {
	Metadata description.CharmMetadata
	Manifest description.CharmManifest
	Actions  description.CharmActions
	Config   description.CharmConfigs
}

// Import the application charm description from the migrating model into
// the current model. This will then be saved with the application and allow
// us to keep RI (referential integrity) of the application and the charm.
func (i *importOperation) importCharm(ctx context.Context, data charmData) (internalcharm.Charm, error) {
	// Don't be tempted to just use the internal/charm package here, or to
	// attempt to make the description package conform to the internal/charm
	// package Charm interface. The internal/charm package is for dealing with
	// charms sent on the wire. These are different things.
	// The description package is versioned at a different rate than the ones
	// in the internal/charm package.
	// By converting the description to the internal charm we can ensure that
	// there is some level of negotiation between the two.
	//
	// This is a good thing.

	metadata, err := i.importCharmMetadata(data.Metadata)
	if err != nil {
		return nil, errors.Errorf("import charm metadata: %w", err)
	}

	manifest, err := i.importCharmManifest(data.Manifest)
	if err != nil {
		return nil, errors.Errorf("import charm manifest: %w", err)
	}

	lxdProfile, err := i.importCharmLXDProfile(data.Metadata)
	if err != nil {
		return nil, errors.Errorf("import charm lxd profile: %w", err)
	}

	config, err := i.importCharmConfig(data.Config)
	if err != nil {
		return nil, errors.Errorf("import charm config: %w", err)
	}

	actions, err := i.importCharmActions(data.Actions)
	if err != nil {
		return nil, errors.Errorf("import charm actions: %w", err)
	}

	// Return a valid charm base that can then be used to create the
	// application.
	return internalcharm.NewCharmBase(metadata, manifest, config, actions, lxdProfile), nil
}

func (i *importOperation) importCharmMetadata(data description.CharmMetadata) (*internalcharm.Meta, error) {
	if data == nil {
		return nil, errors.Errorf("import charm metadata: %w", coreerrors.NotValid)
	}

	var (
		err    error
		runsAs internalcharm.RunAs
	)
	if runsAs, err = importCharmUser(data); err != nil {
		return nil, errors.Errorf("import charm user: %w", err)
	}

	var assumes *assumes.ExpressionTree
	if assumes, err = importAssumes(data.Assumes()); err != nil {
		return nil, errors.Errorf("import charm assumes: %w", err)
	}

	var minJujuVersion semversion.Number
	if minJujuVersion, err = importMinJujuVersion(data.MinJujuVersion()); err != nil {
		return nil, errors.Errorf("import min juju version: %w", err)
	}

	var provides map[string]internalcharm.Relation
	if provides, err = importRelations(data.Provides()); err != nil {
		return nil, errors.Errorf("import provides relations: %w", err)
	}

	var requires map[string]internalcharm.Relation
	if requires, err = importRelations(data.Requires()); err != nil {
		return nil, errors.Errorf("import requires relations: %w", err)
	}

	var peers map[string]internalcharm.Relation
	if peers, err = importRelations(data.Peers()); err != nil {
		return nil, errors.Errorf("import peers relations: %w", err)
	}

	var storage map[string]internalcharm.Storage
	if storage, err = importStorage(data.Storage()); err != nil {
		return nil, errors.Errorf("import storage: %w", err)
	}

	var devices map[string]internalcharm.Device
	if devices, err = importDevices(data.Devices()); err != nil {
		return nil, errors.Errorf("import devices: %w", err)
	}

	var containers map[string]internalcharm.Container
	if containers, err = importContainers(data.Containers()); err != nil {
		return nil, errors.Errorf("import containers: %w", err)
	}

	var resources map[string]resource.Meta
	if resources, err = importResources(data.Resources()); err != nil {
		return nil, errors.Errorf("import resources: %w", err)
	}

	return &internalcharm.Meta{
		Name:           data.Name(),
		Summary:        data.Summary(),
		Description:    data.Description(),
		Subordinate:    data.Subordinate(),
		Categories:     data.Categories(),
		Tags:           data.Tags(),
		Terms:          data.Terms(),
		CharmUser:      runsAs,
		Assumes:        assumes,
		MinJujuVersion: minJujuVersion,
		Provides:       provides,
		Requires:       requires,
		Peers:          peers,
		ExtraBindings:  importExtraBindings(data.ExtraBindings()),
		Storage:        storage,
		Devices:        devices,
		Containers:     containers,
		Resources:      resources,
	}, nil
}

func (i *importOperation) importCharmManifest(data description.CharmManifest) (*internalcharm.Manifest, error) {
	charmBases := data.Bases()
	if data == nil || len(charmBases) == 0 {
		return nil, errors.Errorf("manifest empty")
	}

	bases, err := importManifestBases(charmBases)
	if err != nil {
		return nil, errors.Errorf("import manifest bases: %w", err)
	}
	return &internalcharm.Manifest{
		Bases: bases,
	}, nil
}

func (i *importOperation) importCharmLXDProfile(data description.CharmMetadata) (*internalcharm.LXDProfile, error) {
	// LXDProfile is optional, so if we don't have any data, we can just return
	// nil. If it does exist, then it's JSON encoded blob that we need to
	// unmarshal into the internalcharm.LXDProfile struct.

	lxdProfile := data.LXDProfile()
	if lxdProfile == "" {
		return nil, nil
	}

	var profile internalcharm.LXDProfile
	if err := json.Unmarshal([]byte(lxdProfile), &profile); err != nil {
		return nil, errors.Errorf("unmarshal lxd profile: %w", err)
	}

	return &profile, nil
}

func (i *importOperation) importCharmConfig(data description.CharmConfigs) (*internalcharm.Config, error) {
	// Charm config is optional, so if we don't have any data, we can just
	// return nil.
	if data == nil {
		return nil, nil
	}

	descriptionConfig := data.Configs()

	options := make(map[string]internalcharm.Option, len(descriptionConfig))
	for name, c := range descriptionConfig {
		options[name] = internalcharm.Option{
			Type:        c.Type(),
			Description: c.Description(),
			Default:     c.Default(),
		}
	}

	return &internalcharm.Config{
		Options: options,
	}, nil

}

func (i *importOperation) importCharmActions(data description.CharmActions) (*internalcharm.Actions, error) {
	// Charm actions is optional, so if we don't have any data, we can just
	// return nil.
	if data == nil {
		return nil, nil
	}

	descriptionActions := data.Actions()

	actions := make(map[string]internalcharm.ActionSpec, len(descriptionActions))
	for name, a := range descriptionActions {
		parameters, err := importCharmParameters(a.Parameters())
		if err != nil {
			return nil, errors.Errorf("import charm parameters: %w", err)
		}

		actions[name] = internalcharm.ActionSpec{
			Description:    a.Description(),
			Parallel:       a.Parallel(),
			ExecutionGroup: a.ExecutionGroup(),
			Params:         parameters,
		}
	}

	return &internalcharm.Actions{
		ActionSpecs: actions,
	}, nil
}

func (i *importOperation) importEndpointBindings(app description.Application, spaces []description.Space) (map[string]network.SpaceName, error) {
	endpointBindings := make(map[string]network.SpaceName)
	// Get the application's default space
	applicationDefaultSpaceID := network.AlphaSpaceName
	for endpoint, spaceID := range app.EndpointBindings() {
		if endpoint == "" {
			applicationDefaultSpaceID = spaceID
			break
		}
	}

	// Find the spaces names associated with each endpoint.
	for endpoint, spaceID := range app.EndpointBindings() {
		if spaceID == "" || (spaceID == applicationDefaultSpaceID && endpoint != "") {
			// In migrations from 4.0+, an empty space ID signifies that the
			// endpoint is set to the applications default space. Any endpoints
			// not included explicitly in create application will be set to the
			// application default, so do not set it here.

			// Similarly, if the spaceID is the application default space, then
			// don't add insert it. In 3.6 endpoints had their spaceID
			// explicitly set to the default space.
			continue
		} else if spaceID == network.AlphaSpaceId || spaceID == "0" {
			// If the space ID is that of the alpha space, then bind the
			// endpoint to the alpha space name.
			endpointBindings[endpoint] = network.AlphaSpaceName
		} else {
			// Search through the imported spaceIDs to find the corresponding
			// space name for the endpoint binding.
			var spaceName string
			for _, spaceInfo := range spaces {
				if spaceInfo.Id() == spaceID {
					spaceName = spaceInfo.Name()
					break
				} else if spaceInfo.UUID() == spaceID {
					// This means that the space was inserted from a 4.0+ model.
					spaceName = spaceInfo.Name()
					break
				}
			}
			if spaceName == "" {
				return nil, errors.Errorf("space with id %q not found", spaceID)
			}
			endpointBindings[endpoint] = network.SpaceName(spaceName)
		}
	}
	return endpointBindings, nil
}

func (i *importOperation) importExposedEndpoints(ctx context.Context, app description.Application, spaces []description.Space) (map[string]application.ExposedEndpoint, error) {
	if !app.Exposed() {
		return make(map[string]application.ExposedEndpoint), nil
	}

	exposedEndpoints := make(map[string]application.ExposedEndpoint)
	for endpoint, exposedEndpoint := range app.ExposedEndpoints() {
		// Since pre-4.0 spaces had only an Id (and not a UUID) set, we need to
		// map the exposed endpoints to the spaces to the new UUIDs as inserted
		// by the network domain. We do this implicit mapping by looking the
		// spaces by name.
		exposedToSpaceUUIDs := make([]string, 0, len(exposedEndpoint.ExposeToSpaceIDs()))
		for _, spaceID := range exposedEndpoint.ExposeToSpaceIDs() {
			// We don't export the alpha space to the description model, so we
			// must verify if the space ID is either the 4.0+ alpha space ID or
			// the legacy ID ("0"). In that case we just map it to the new alpha
			// space UUID.
			//
			if spaceID == network.AlphaSpaceId || spaceID == "0" {
				exposedToSpaceUUIDs = append(exposedToSpaceUUIDs, network.AlphaSpaceId)
				continue
			}

			var spaceName string
			for _, spaceInfo := range spaces {
				if spaceInfo.Id() == spaceID {
					spaceName = spaceInfo.Name()
					break
				} else if spaceInfo.UUID() == spaceID {
					// This means that the space was inserted from a 4.0+ model.
					spaceName = spaceInfo.Name()
					break
				}
			}

			if spaceName == "" {
				return nil, errors.Errorf("endpoint exposed to space %q does not exist", spaceID)
			}
			spaceUUID, err := i.service.GetSpaceUUIDByName(ctx, spaceName)
			if err != nil {
				return nil, errors.Errorf("getting space UUID by name %q: %w", spaceID, err)
			}
			exposedToSpaceUUIDs = append(exposedToSpaceUUIDs, spaceUUID.String())
		}
		exposedEndpoints[endpoint] = application.ExposedEndpoint{
			ExposeToCIDRs:    set.NewStrings(exposedEndpoint.ExposeToCIDRs()...),
			ExposeToSpaceIDs: set.NewStrings(exposedToSpaceUUIDs...),
		}
	}
	return exposedEndpoints, nil
}

func importCharmUser(data description.CharmMetadata) (internalcharm.RunAs, error) {
	switch data.RunAs() {
	case runAsDefault, "":
		return internalcharm.RunAsDefault, nil
	case runAsRoot:
		return internalcharm.RunAsRoot, nil
	case runAsSudoer:
		return internalcharm.RunAsSudoer, nil
	case runAsNonRoot:
		return internalcharm.RunAsNonRoot, nil
	default:
		return internalcharm.RunAsDefault, errors.Errorf("unknown run-as value %q: %w", data.RunAs(), coreerrors.NotValid)
	}
}

func importAssumes(data string) (*assumes.ExpressionTree, error) {
	// Assumes is a recursive structure, rather than sending all that data over
	// the wire as yaml, the description package encodes that information as
	// a JSON string.

	// If the data is empty, we don't have any assumes at all, so it's safe
	// to just return nil.
	if data == "" {
		return nil, nil
	}

	tree := new(assumes.ExpressionTree)
	if err := tree.UnmarshalJSON([]byte(data)); err != nil {
		return nil, errors.Errorf("unmarshal assumes: %w: %w", err, coreerrors.NotValid)
	}
	return tree, nil
}

func importMinJujuVersion(data string) (semversion.Number, error) {
	// minJujuVersion is optional, so if the data is empty, we can just return
	// an empty version.
	if data == "" {
		return semversion.Number{}, nil
	}

	ver, err := semversion.Parse(data)
	if err != nil {
		return semversion.Number{}, errors.Errorf("parse min juju version: %w: %w", err, coreerrors.NotValid)
	}
	return ver, nil
}

func importRelations(data map[string]description.CharmMetadataRelation) (map[string]internalcharm.Relation, error) {
	relations := make(map[string]internalcharm.Relation, len(data))
	for name, rel := range data {
		role, err := importRelationRole(rel.Role())
		if err != nil {
			return nil, errors.Errorf("import relation role: %w", err)
		}

		scope, err := importRelationScope(rel.Scope())
		if err != nil {
			return nil, errors.Errorf("import relation scope: %w", err)
		}

		relations[name] = internalcharm.Relation{
			Name:      rel.Name(),
			Role:      role,
			Interface: rel.Interface(),
			Optional:  rel.Optional(),
			Limit:     rel.Limit(),
			Scope:     scope,
		}
	}
	return relations, nil
}

func importRelationRole(data string) (internalcharm.RelationRole, error) {
	switch data {
	case rolePeer:
		return internalcharm.RolePeer, nil
	case roleProvider:
		return internalcharm.RoleProvider, nil
	case roleRequirer:
		return internalcharm.RoleRequirer, nil
	default:
		return "", errors.Errorf("unknown relation role %q: %w", data, coreerrors.NotValid)
	}
}

func importRelationScope(data string) (internalcharm.RelationScope, error) {
	switch data {
	case scopeGlobal:
		return internalcharm.ScopeGlobal, nil
	case scopeContainer:
		return internalcharm.ScopeContainer, nil
	default:
		return "", errors.Errorf("unknown relation scope %q: %w", data, coreerrors.NotValid)
	}
}

func importExtraBindings(data map[string]string) map[string]internalcharm.ExtraBinding {
	extraBindings := make(map[string]internalcharm.ExtraBinding, len(data))
	for key, name := range data {
		extraBindings[key] = internalcharm.ExtraBinding{
			Name: name,
		}
	}
	return extraBindings
}

func importStorage(data map[string]description.CharmMetadataStorage) (map[string]internalcharm.Storage, error) {
	storage := make(map[string]internalcharm.Storage, len(data))
	for name, s := range data {
		typ, err := importStorageType(s.Type())
		if err != nil {
			return nil, errors.Errorf("import storage type: %w", err)
		}

		storage[name] = internalcharm.Storage{
			Name:        s.Name(),
			Type:        typ,
			Description: s.Description(),
			Shared:      s.Shared(),
			ReadOnly:    s.Readonly(),
			MinimumSize: uint64(s.MinimumSize()),
			Location:    s.Location(),
			CountMin:    s.CountMin(),
			CountMax:    s.CountMax(),
			Properties:  s.Properties(),
		}
	}
	return storage, nil
}

func importStorageType(data string) (internalcharm.StorageType, error) {
	switch data {
	case storageBlock:
		return internalcharm.StorageBlock, nil
	case storageFilesystem:
		return internalcharm.StorageFilesystem, nil
	default:
		return "", errors.Errorf("unknown storage type %q: %w", data, coreerrors.NotValid)
	}
}

func importDevices(data map[string]description.CharmMetadataDevice) (map[string]internalcharm.Device, error) {
	devices := make(map[string]internalcharm.Device, len(data))
	for name, d := range data {

		devices[name] = internalcharm.Device{
			Name:        d.Name(),
			Description: d.Description(),
			Type:        internalcharm.DeviceType(d.Type()),
			CountMin:    int64(d.CountMin()),
			CountMax:    int64(d.CountMax()),
		}
	}
	return devices, nil
}

func importContainers(data map[string]description.CharmMetadataContainer) (map[string]internalcharm.Container, error) {
	containers := make(map[string]internalcharm.Container, len(data))
	for name, c := range data {
		mounts := make([]internalcharm.Mount, len(c.Mounts()))
		for i, m := range c.Mounts() {
			mounts[i] = internalcharm.Mount{
				Location: m.Location(),
				Storage:  m.Storage(),
			}
		}

		containers[name] = internalcharm.Container{
			Resource: c.Resource(),
			Uid:      c.Uid(),
			Gid:      c.Gid(),
			Mounts:   mounts,
		}
	}
	return containers, nil
}

func importResources(data map[string]description.CharmMetadataResource) (map[string]resource.Meta, error) {
	resources := make(map[string]resource.Meta, len(data))
	for name, r := range data {
		typ, err := importResourceType(r.Type())
		if err != nil {
			return nil, errors.Errorf("import resource type: %w", err)
		}

		resources[name] = resource.Meta{
			Name:        r.Name(),
			Type:        typ,
			Path:        r.Path(),
			Description: r.Description(),
		}
	}
	return resources, nil
}

func importResourceType(data string) (resource.Type, error) {
	switch data {
	case resourceFile:
		return resource.TypeFile, nil
	case resourceContainer:
		return resource.TypeContainerImage, nil
	default:
		return -1, errors.Errorf("unknown resource type %q: %w", data, coreerrors.NotValid)
	}
}

func importManifestBases(data []description.CharmManifestBase) ([]internalcharm.Base, error) {
	// This shouldn't happen, but we should handle the case that if we don't
	// have any bases, we should just return nil.
	if len(data) == 0 {
		return nil, nil
	}

	bases := make([]internalcharm.Base, len(data))
	for i, base := range data {
		channel, err := importBaseChannel(base.Channel())
		if err != nil {
			return nil, errors.Errorf("import channel for %q: %w", base.Name(), err)
		}

		bases[i] = internalcharm.Base{
			Name:          base.Name(),
			Channel:       channel,
			Architectures: base.Architectures(),
		}
	}
	return bases, nil
}

func importBaseChannel(data string) (internalcharm.Channel, error) {
	// We expect the channel to be non-empty. The parse channel will return
	// not valid error if it is empty. This might be a bit too strict, but
	// it's better to be strict than to be lenient.
	return internalcharm.ParseChannel(data)
}

func ptr[T any](v T) *T {
	return &v
}

func importCharmParameters(parameters map[string]any) (map[string]any, error) {
	if len(parameters) == 0 {
		return nil, nil
	}

	// We can't have any nested maps that are of the type map[any]any, so we
	// need to convert the map[any]any to map[string]any.
	result := make(map[string]any, len(parameters))
	for key, value := range parameters {
		switch value := value.(type) {
		case map[any]any:
			nested, err := convertNestedMap(value)
			if err != nil {
				return nil, errors.Errorf("convert nested map: %w", err)
			}
			result[key] = nested
		default:
			result[key] = value
		}
	}
	return result, nil
}

// convertNestedMap converts a nested map[any]any to a map[string]any.
// This is a recursive function that will convert all nested maps to
// map[string]any.
func convertNestedMap(nested map[any]any) (map[string]any, error) {
	if len(nested) == 0 {
		return nil, nil
	}

	result := make(map[string]any, len(nested))
	for key, value := range nested {
		coercedKey, err := convertKey(key)
		if err != nil {
			return nil, errors.Errorf("convert key %v: %w", key, err)
		}

		switch value := value.(type) {
		case map[any]any:
			nested, err := convertNestedMap(value)
			if err != nil {
				return nil, errors.Errorf("convert nested map: %w", err)
			}
			result[coercedKey] = nested
		default:
			result[coercedKey] = value
		}
	}
	return result, nil
}

func convertKey(key any) (string, error) {
	switch key := key.(type) {
	case string:
		return key, nil
	case fmt.Stringer:
		return key.String(), nil
	case int:
		return strconv.Itoa(key), nil
	case int64:
		return strconv.FormatInt(key, 10), nil
	case float64:
		return strconv.FormatFloat(key, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(key), nil
	default:
		return "", errors.Errorf("key can not be converted to a string: %w", coreerrors.NotValid)
	}
}
