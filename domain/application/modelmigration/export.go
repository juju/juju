// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/description/v9"

	coreapplication "github.com/juju/juju/core/application"
	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/os/ostype"
	corestorage "github.com/juju/juju/core/storage"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// RegisterExport registers the export operations with the given coordinator.
func RegisterExport(
	coordinator Coordinator,
	registry corestorage.ModelStorageRegistryGetter,
	clock clock.Clock,
	logger logger.Logger,
) {
	coordinator.Add(&exportOperation{
		registry: registry,
		clock:    clock,
		logger:   logger,
	})
}

// ExportService provides a subset of the application domain
// service methods needed for application export.
type ExportService interface {
	// ExportApplications returns all the applications in the model.
	GetApplicationsForExport(ctx context.Context) ([]application.ExportApplication, error)

	// GetCharmID returns a charm ID by name. It returns an error.CharmNotFound
	// if the charm can not be found by the name.
	// This can also be used as a cheap way to see if a charm exists without
	// needing to load the charm metadata.
	GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error)

	// GetCharmByApplicationName returns the charm metadata for the given charm
	// ID. It returns an error.CharmNotFound if the charm can not be found by
	// the ID.
	GetCharmByApplicationName(ctx context.Context, name string) (internalcharm.Charm, charm.CharmLocator, error)

	// GetApplicationConfigAndSettings returns the application config and
	// settings for the specified application. This will return the application
	// config and the settings in one config.ConfigAttributes object.
	//
	// If the application does not exist, a
	// [applicationerrors.ApplicationNotFound] error is returned. If no config
	// is set for the application, an empty config is returned.
	GetApplicationConfigAndSettings(ctx context.Context, name string) (config.ConfigAttributes, application.ApplicationSettings, error)

	// GetApplicationConstraints returns the application constraints for the
	// specified application name.
	// Empty constraints are returned if no constraints exist for the given
	// application ID.
	// If no application is found, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationConstraints(ctx context.Context, name string) (constraints.Value, error)

	// GetUnitUUIDByName returns the unit UUID for the specified unit name.
	// If the unit does not exist, an error satisfying
	// [applicationerrors.UnitNotFound] is returned.
	GetUnitUUIDByName(ctx context.Context, name coreunit.Name) (coreunit.UUID, error)

	// GetApplicationScaleState returns the scale state of the specified
	// application, returning an error satisfying
	// [applicationerrors.ApplicationNotFound] if the application is not found.
	GetApplicationScaleState(ctx context.Context, name string) (application.ScaleState, error)

	// GetApplicationCharmOrigin returns the charm origin for the specified
	// application name.
	// If the application does not exist, an error satisfying
	// [applicationerrors.ApplicationNotFound] is returned.
	GetApplicationCharmOrigin(ctx context.Context, name string) (application.CharmOrigin, error)
}

// exportOperation describes a way to execute a migration for
// exporting applications.
type exportOperation struct {
	modelmigration.BaseOperation

	service ExportService

	registry corestorage.ModelStorageRegistryGetter
	clock    clock.Clock
	logger   logger.Logger
}

// Name returns the name of this operation.
func (e *exportOperation) Name() string {
	return "export applications"
}

// Setup the export operation.
// This will create a new service instance.
func (e *exportOperation) Setup(scope modelmigration.Scope) error {
	e.service = service.NewMigrationService(
		state.NewState(scope.ModelDB(), e.clock, e.logger),
		e.registry,
		e.clock,
		e.logger,
	)
	return nil
}

// Execute the export, adding the application to the model.
// The export also includes all the charm metadata, manifest, config and
// actions. Along with units and resources.
func (e *exportOperation) Execute(ctx context.Context, model description.Model) error {
	applications, err := e.service.GetApplicationsForExport(ctx)
	if err != nil {
		return errors.Errorf("getting applications for export: %w", err)
	}

	for _, app := range applications {
		appArgs, err := e.createApplicationArgs(ctx, app)
		if err != nil {
			return errors.Errorf("creating application args: %w", err)
		}

		// Create the application in the description model. This returns the
		// application description, which we can then populate with the
		// additional data.
		descriptionApp := model.AddApplication(appArgs)

		// Get the application constraints and set them on the application.
		appCons, err := e.service.GetApplicationConstraints(ctx, app.Name)
		if err != nil {
			return errors.Errorf("getting application constraints %q: %w", app.Name, err)
		}
		descriptionApp.SetConstraints(exportApplicationConstraints(appCons))

		// Set the charm origin on the application.
		origin, err := e.service.GetApplicationCharmOrigin(ctx, app.Name)
		if err != nil {
			return errors.Errorf("getting application charm origin for %q: %w", app.Name, err)
		}
		if err := e.exportApplicationCharmOrigin(ctx, descriptionApp, origin, appCons); err != nil {
			return errors.Capture(err)
		}

		// Set the application config and settings.
		config, settings, err := e.service.GetApplicationConfigAndSettings(ctx, app.Name)
		if err != nil {
			return errors.Errorf("getting application config for %q: %w", app.Name, err)
		}

		// The naming of these methods are esoteric, essentially the charm
		// config is the application config overlaid from the charm config. The
		// application config, is the application settings.
		descriptionApp.SetCharmConfig(config)
		descriptionApp.SetApplicationConfig(map[string]any{
			coreapplication.TrustConfigOptionName: settings.Trust,
		})

		charm, _, err := e.service.GetCharmByApplicationName(ctx, app.Name)
		if err != nil {
			return errors.Errorf("getting charm: %w", err)
		}

		if err := e.exportCharm(ctx, descriptionApp, charm); err != nil {
			return errors.Capture(err)
		}

		scaleState, err := e.service.GetApplicationScaleState(ctx, app.Name)
		if err != nil {
			return errors.Errorf("getting application scale state for %q: %w", app.Name, err)
		}
		descriptionApp.SetProvisioningState(exportApplicationScaleState(scaleState))
		descriptionApp.SetDesiredScale(scaleState.Scale)

		err = e.exportApplicationUnits(ctx, descriptionApp)
		if err != nil {
			return errors.Errorf("exporting application units %q: %w", app.Name, err)
		}
	}

	return nil
}

func (e *exportOperation) createApplicationArgs(ctx context.Context, app application.ExportApplication) (description.ApplicationArgs, error) {
	modelType, err := e.exportModelType(app.ModelType)
	if err != nil {
		return description.ApplicationArgs{}, errors.Capture(err)
	}

	charmURL, err := exportCharmURL(app.CharmLocator)
	if err != nil {
		return description.ApplicationArgs{}, errors.Capture(err)
	}

	return description.ApplicationArgs{
		Name:                 app.Name,
		Type:                 modelType,
		Subordinate:          app.Subordinate,
		CharmURL:             charmURL,
		CharmModifiedVersion: app.CharmModifiedVersion,
		ForceCharm:           app.CharmUpgradeOnError,
		Exposed:              app.Exposed,
		PasswordHash:         app.PasswordHash,
		Placement:            app.Placement,
	}, nil
}

func (e *exportOperation) exportModelType(modelType model.ModelType) (string, error) {
	switch modelType {
	case model.IAAS:
		return "iaas", nil
	case model.CAAS:
		return "caas", nil
	default:
		return "", errors.Errorf("unsupported model type %q", modelType)
	}
}

func (e *exportOperation) exportApplicationCharmOrigin(
	ctx context.Context,
	app description.Application, origin application.CharmOrigin,
	cons constraints.Value,
) error {
	source, err := exportSource(origin.Source)
	if err != nil {
		return errors.Capture(err)
	}

	channel, err := exportChannel(origin.Channel)
	if err != nil {
		return errors.Capture(err)
	}

	defaultArch := arch.DefaultArchitecture
	if cons.HasArch() && *cons.Arch != "" {
		defaultArch = *cons.Arch
	}

	platform, err := e.exportPlatform(ctx, origin.Platform, defaultArch)
	if err != nil {
		return errors.Capture(err)
	}

	originArgs := description.CharmOriginArgs{
		Source:   source,
		ID:       origin.CharmhubIdentifier,
		Revision: origin.Revision,
		Hash:     origin.Hash,
		Channel:  channel,
		Platform: platform,
	}

	app.SetCharmOrigin(originArgs)
	return nil
}

func exportChannel(channel *application.Channel) (string, error) {
	if channel == nil {
		return "", nil
	}

	risk, err := exportRisk(channel.Risk)
	if err != nil {
		return "", errors.Capture(err)
	}

	ch := internalcharm.MakePermissiveChannel(channel.Track, risk, channel.Branch)
	return ch.String(), nil
}

func exportRisk(risk application.ChannelRisk) (string, error) {
	switch risk {
	case application.RiskStable:
		return "stable", nil
	case application.RiskCandidate:
		return "candidate", nil
	case application.RiskBeta:
		return "beta", nil
	case application.RiskEdge:
		return "edge", nil
	default:
		return "", errors.Errorf("unsupported risk %q", risk)
	}
}

func (e *exportOperation) exportPlatform(ctx context.Context, platform application.Platform, defaultArch string) (string, error) {
	arch := e.exportArchitecture(ctx, platform.Architecture, defaultArch)

	os, err := exportOSType(platform.OSType)
	if err != nil {
		return "", errors.Capture(err)
	}

	p := corecharm.Platform{
		Architecture: arch,
		OS:           os,
		Channel:      platform.Channel,
	}
	return p.String(), nil
}

func (e *exportOperation) exportArchitecture(ctx context.Context, a application.Architecture, defaultArch string) string {
	switch a {
	case architecture.AMD64:
		return arch.AMD64
	case architecture.ARM64:
		return arch.ARM64
	case architecture.PPC64EL:
		return arch.PPC64EL
	case architecture.S390X:
		return arch.S390X
	case architecture.RISCV64:
		return arch.RISCV64
	default:
		e.logger.Warningf(ctx, "no architecture set for platform, using default %q", defaultArch)
		return defaultArch
	}
}

func exportOSType(osType application.OSType) (string, error) {
	switch osType {
	case application.Ubuntu:
		return ostype.Ubuntu.String(), nil
	default:
		return "", errors.Errorf("unsupported os type %q", osType)
	}
}

func exportApplicationScaleState(scaleState application.ScaleState) *description.ProvisioningStateArgs {
	return &description.ProvisioningStateArgs{
		Scaling:     scaleState.Scaling,
		ScaleTarget: scaleState.ScaleTarget,
	}
}

func exportApplicationConstraints(cons constraints.Value) description.ConstraintsArgs {
	result := description.ConstraintsArgs{}
	if cons.AllocatePublicIP != nil {
		result.AllocatePublicIP = *cons.AllocatePublicIP
	}
	if cons.Arch != nil {
		result.Architecture = *cons.Arch
	}
	if cons.Container != nil {
		result.Container = string(*cons.Container)
	}
	if cons.CpuCores != nil {
		result.CpuCores = *cons.CpuCores
	}
	if cons.CpuPower != nil {
		result.CpuPower = *cons.CpuPower
	}
	if cons.Mem != nil {
		result.Memory = *cons.Mem
	}
	if cons.RootDisk != nil {
		result.RootDisk = *cons.RootDisk
	}
	if cons.RootDiskSource != nil {
		result.RootDiskSource = *cons.RootDiskSource
	}
	if cons.ImageID != nil {
		result.ImageID = *cons.ImageID
	}
	if cons.InstanceType != nil {
		result.InstanceType = *cons.InstanceType
	}
	if cons.VirtType != nil {
		result.VirtType = *cons.VirtType
	}
	if cons.Spaces != nil {
		result.Spaces = *cons.Spaces
	}
	if cons.Tags != nil {
		result.Tags = *cons.Tags
	}
	if cons.Zones != nil {
		result.Zones = *cons.Zones
	}
	return result
}

// exportCharmURL returns the charm URL for the current model.
func exportCharmURL(locator charm.CharmLocator) (string, error) {
	schema, err := exportSource(locator.Source)
	if err != nil {
		return "", errors.Capture(err)
	}

	architecture, err := exportCharmURLArchitecture(locator.Architecture)
	if err != nil {
		return "", errors.Capture(err)
	}

	url := internalcharm.URL{
		Schema:       schema,
		Name:         locator.Name,
		Revision:     locator.Revision,
		Architecture: architecture,
	}
	return url.String(), nil
}

func exportSource(source charm.CharmSource) (string, error) {
	switch source {
	case charm.CharmHubSource:
		return internalcharm.CharmHub.String(), nil
	case charm.LocalSource:
		return internalcharm.Local.String(), nil
	default:
		return "", errors.Errorf("unsupported source %q", source)
	}
}

func exportCharmURLArchitecture(a application.Architecture) (string, error) {
	switch a {
	case architecture.AMD64:
		return arch.AMD64, nil
	case architecture.ARM64:
		return arch.ARM64, nil
	case architecture.PPC64EL:
		return arch.PPC64EL, nil
	case architecture.S390X:
		return arch.S390X, nil
	case architecture.RISCV64:
		return arch.RISCV64, nil

	// This is a valid case if we're uploading charms and the value isn't
	// supplied.
	case architecture.Unknown:
		return "", nil
	default:
		return "", errors.Errorf("unsupported architecture %q", a)
	}
}

const (
	// Convert the charm-user to a string representation. This is a string
	// representation of the internalcharm.RunAs type. This is done to ensure
	// that if any changes to the on the wire protocol are made, we can easily
	// adapt and convert to them, without breaking migrations to older versions.
	// The strings ARE the API when it comes to migrations.
	runAsRoot    = "root"
	runAsDefault = "default"
	runAsNonRoot = "non-root"
	runAsSudoer  = "sudoer"
)

func exportCharmUser(user internalcharm.RunAs) (string, error) {
	switch user {
	case internalcharm.RunAsRoot:
		return runAsRoot, nil
	case internalcharm.RunAsDefault:
		return runAsDefault, nil
	case internalcharm.RunAsNonRoot:
		return runAsNonRoot, nil
	case internalcharm.RunAsSudoer:
		return runAsSudoer, nil
	default:
		return "", errors.Errorf("unknown run-as value %q", user)
	}
}

func exportRelations(relations map[string]internalcharm.Relation) (map[string]description.CharmMetadataRelation, error) {
	result := make(map[string]description.CharmMetadataRelation, len(relations))
	for name, relation := range relations {
		args, err := exportRelation(relation)
		if err != nil {
			return nil, errors.Capture(err)
		}
		result[name] = args
	}
	return result, nil
}

func exportRelation(relation internalcharm.Relation) (description.CharmMetadataRelation, error) {
	role, err := exportCharmRole(relation.Role)
	if err != nil {
		return nil, errors.Capture(err)
	}

	scope, err := exportCharmScope(relation.Scope)
	if err != nil {
		return nil, errors.Capture(err)
	}

	return relationType{
		name:     relation.Name,
		role:     role,
		iface:    relation.Interface,
		optional: relation.Optional,
		limit:    relation.Limit,
		scope:    scope,
	}, nil
}

const (
	// Convert the charm role to a string representation. This is a string
	// representation of the internalcharm.RelationRole type. This is done to
	// ensure that if any changes to the on the wire protocol are made, we can
	// easily adapt and convert to them, without breaking migrations to older
	// versions. The strings ARE the API when it comes to migrations.
	roleProvider = "provider"
	roleRequirer = "requirer"
	rolePeer     = "peer"
)

func exportCharmRole(role internalcharm.RelationRole) (string, error) {
	switch role {
	case internalcharm.RoleProvider:
		return roleProvider, nil
	case internalcharm.RoleRequirer:
		return roleRequirer, nil
	case internalcharm.RolePeer:
		return rolePeer, nil
	default:
		return "", errors.Errorf("unknown role value %q", role)
	}
}

const (
	// Convert the charm scope to a string representation. This is a string
	// representation of the internalcharm.RelationScope type. This is done to
	// ensure that if any changes to the on the wire protocol are made, we can
	// easily adapt and convert to them, without breaking migrations to older
	// versions. The strings ARE the API when it comes to migrations.
	scopeGlobal    = "global"
	scopeContainer = "container"
)

func exportCharmScope(scope internalcharm.RelationScope) (string, error) {
	switch scope {
	case internalcharm.ScopeGlobal:
		return scopeGlobal, nil
	case internalcharm.ScopeContainer:
		return scopeContainer, nil
	default:
		return "", errors.Errorf("unknown scope value %q", scope)
	}
}

func exportExtraBindings(bindings map[string]internalcharm.ExtraBinding) map[string]string {
	result := make(map[string]string, len(bindings))
	for key, value := range bindings {
		result[key] = value.Name
	}
	return result
}

func exportStorage(storage map[string]internalcharm.Storage) (map[string]description.CharmMetadataStorage, error) {
	result := make(map[string]description.CharmMetadataStorage, len(storage))
	for name, storage := range storage {
		typ, err := exportStorageType(storage)
		if err != nil {
			return nil, errors.Capture(err)
		}

		result[name] = storageType{
			name:        storage.Name,
			description: storage.Description,
			typ:         typ,
			shared:      storage.Shared,
			readonly:    storage.ReadOnly,
			countMin:    storage.CountMin,
			countMax:    storage.CountMax,
			minimumSize: int(storage.MinimumSize),
			location:    storage.Location,
			properties:  storage.Properties,
		}
	}
	return result, nil
}

const (
	// Convert the charm storage type to a string representation. This is a string
	// representation of the internalcharm.StorageType type. This is done to
	// ensure that if any changes to the on the wire protocol are made, we can
	// easily adapt and convert to them, without breaking migrations to older
	// versions. The strings ARE the API when it comes to migrations.
	storageBlock      = "block"
	storageFilesystem = "filesystem"
)

func exportStorageType(storage internalcharm.Storage) (string, error) {
	switch storage.Type {
	case internalcharm.StorageBlock:
		return storageBlock, nil
	case internalcharm.StorageFilesystem:
		return storageFilesystem, nil
	default:
		return "", errors.Errorf("unknown storage type %q", storage.Type)
	}
}

func exportDevices(devices map[string]internalcharm.Device) (map[string]description.CharmMetadataDevice, error) {
	result := make(map[string]description.CharmMetadataDevice, len(devices))
	for name, device := range devices {
		result[name] = deviceType{
			name:        device.Name,
			description: device.Description,
			typ:         string(device.Type),
			countMin:    int(device.CountMin),
			countMax:    int(device.CountMax),
		}
	}
	return result, nil
}

func exportContainers(containers map[string]internalcharm.Container) (map[string]description.CharmMetadataContainer, error) {
	result := make(map[string]description.CharmMetadataContainer, len(containers))
	for name, container := range containers {
		mounts := exportContainerMounts(container.Mounts)

		result[name] = containerType{
			resource: container.Resource,
			mounts:   mounts,
			uid:      container.Uid,
			gid:      container.Gid,
		}
	}
	return result, nil
}

func exportContainerMounts(mounts []internalcharm.Mount) []description.CharmMetadataContainerMount {
	result := make([]description.CharmMetadataContainerMount, len(mounts))
	for i, mount := range mounts {
		result[i] = containerMountType{
			location: mount.Location,
			storage:  mount.Storage,
		}
	}
	return result
}

func exportResources(resources map[string]resource.Meta) (map[string]description.CharmMetadataResource, error) {
	result := make(map[string]description.CharmMetadataResource, len(resources))
	for name, resource := range resources {
		typ, err := exportResourceType(resource.Type)
		if err != nil {
			return nil, errors.Capture(err)
		}

		result[name] = resourceType{
			name:        resource.Name,
			typ:         typ,
			path:        resource.Path,
			description: resource.Description,
		}
	}
	return result, nil
}

const (
	// Convert the charm resource type to a string representation. This is a
	// string representation of the resource.Type type. This is done to ensure
	// that if any changes to the on the wire protocol are made, we can easily
	// adapt and convert to them, without breaking migrations to older versions.
	// The strings ARE the API when it comes to migrations.
	resourceFile      = "file"
	resourceContainer = "oci-image"
)

func exportResourceType(typ resource.Type) (string, error) {
	switch typ {
	case resource.TypeFile:
		return resourceFile, nil
	case resource.TypeContainerImage:
		return resourceContainer, nil
	default:
		return "", errors.Errorf("unknown resource type %q", typ)
	}
}

func exportManifestBases(bases []internalcharm.Base) ([]description.CharmManifestBase, error) {
	result := make([]description.CharmManifestBase, len(bases))
	for i, base := range bases {
		result[i] = baseType{
			name: base.Name,
			// This is potentially dangerous, as we're assuming that the
			// channel does not differ between releases. It's probably wise
			// to normalize this into a model migration version. One that
			// we can ensure is consistent between releases.
			channel:       base.Channel.String(),
			architectures: base.Architectures,
		}
	}
	return result, nil
}

type relationType struct {
	name     string
	role     string
	iface    string
	optional bool
	limit    int
	scope    string
}

// Name returns the name of the relation.
func (r relationType) Name() string {
	return r.name
}

// Role returns the role of the relation.
func (r relationType) Role() string {
	return r.role
}

// Interface returns the interface of the relation.
func (r relationType) Interface() string {
	return r.iface
}

// Optional returns whether the relation is optional.
func (r relationType) Optional() bool {
	return r.optional
}

// Limit returns the limit of the relation.
func (r relationType) Limit() int {
	return r.limit
}

// Scope returns the scope of the relation.
func (r relationType) Scope() string {
	return r.scope
}

type storageType struct {
	name        string
	description string
	typ         string
	shared      bool
	readonly    bool
	countMin    int
	countMax    int
	minimumSize int
	location    string
	properties  []string
}

// Name returns the name of the storage.
func (s storageType) Name() string {
	return s.name
}

// Description returns the description of the storage.
func (s storageType) Description() string {
	return s.description
}

// Type returns the type of the storage.
func (s storageType) Type() string {
	return s.typ
}

// Shared returns whether the storage is shared.
func (s storageType) Shared() bool {
	return s.shared
}

// Readonly returns whether the storage is readonly.
func (s storageType) Readonly() bool {
	return s.readonly
}

// CountMin returns the minimum count of the storage.
func (s storageType) CountMin() int {
	return s.countMin
}

// CountMax returns the maximum count of the storage.
func (s storageType) CountMax() int {
	return s.countMax
}

// MinimumSize returns the minimum size of the storage.
func (s storageType) MinimumSize() int {
	return s.minimumSize
}

// Location returns the location of the storage.
func (s storageType) Location() string {
	return s.location
}

// Properties returns the properties of the storage.
func (s storageType) Properties() []string {
	return s.properties
}

type deviceType struct {
	name        string
	description string
	typ         string
	countMin    int
	countMax    int
}

// Name returns the name of the device.
func (d deviceType) Name() string {
	return d.name
}

// Description returns the description of the device.
func (d deviceType) Description() string {
	return d.description
}

// Type returns the type of the device.
func (d deviceType) Type() string {
	return d.typ
}

// CountMin returns the minimum count of the device.
func (d deviceType) CountMin() int {
	return d.countMin
}

// CountMax returns the maximum count of the device.
func (d deviceType) CountMax() int {
	return d.countMax
}

type containerType struct {
	resource string
	mounts   []description.CharmMetadataContainerMount
	uid      *int
	gid      *int
}

// Resource returns the resource of the container.
func (c containerType) Resource() string {
	return c.resource
}

// Mounts returns the mounts of the container.
func (c containerType) Mounts() []description.CharmMetadataContainerMount {
	return c.mounts
}

// Uid returns the uid of the container.
func (c containerType) Uid() *int {
	return c.uid
}

// Gid returns the gid of the container.
func (c containerType) Gid() *int {
	return c.gid
}

type containerMountType struct {
	location string
	storage  string
}

// Location returns the location of the container mount.
func (c containerMountType) Location() string {
	return c.location
}

// Storage returns the storage of the container mount.
func (c containerMountType) Storage() string {
	return c.storage
}

type resourceType struct {
	name        string
	typ         string
	path        string
	description string
}

// Name returns the name of the resource.
func (r resourceType) Name() string {
	return r.name
}

// Type returns the type of the resource.
func (r resourceType) Type() string {
	return r.typ
}

// Path returns the path of the resource.
func (r resourceType) Path() string {
	return r.path
}

// Description returns the description of the resource.
func (r resourceType) Description() string {
	return r.description
}

type baseType struct {
	name          string
	channel       string
	architectures []string
}

// Name returns the name of the base.
func (b baseType) Name() string {
	return b.name
}

// Channel returns the channel of the base.
func (b baseType) Channel() string {
	return b.channel
}

// Architectures returns the architectures of the base.
func (b baseType) Architectures() []string {
	return b.architectures
}

type configType struct {
	typ          string
	description  string
	defaultValue interface{}
}

// Type returns the type of the config.
func (c configType) Type() string {
	return c.typ
}

// Default returns the default value of the config.
func (c configType) Default() interface{} {
	return c.defaultValue
}

// Description returns the description of the config.
func (c configType) Description() string {
	return c.description
}

type actionType struct {
	description    string
	parallel       bool
	executionGroup string
	parameters     map[string]interface{}
}

// Description returns the description of the action.
func (a actionType) Description() string {
	return a.description
}

// Parallel returns whether the action is parallel.
func (a actionType) Parallel() bool {
	return a.parallel
}

// ExecutionGroup returns the execution group of the action.
func (a actionType) ExecutionGroup() string {
	return a.executionGroup
}

// Parameters returns the parameters of the action.
func (a actionType) Parameters() map[string]interface{} {
	return a.parameters
}
