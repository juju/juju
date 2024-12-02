// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"encoding/json"

	"github.com/juju/clock"
	"github.com/juju/description/v8"

	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	corestorage "github.com/juju/juju/core/storage"
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
	// GetCharmID returns a charm ID by name. It returns an error.CharmNotFound
	// if the charm can not be found by the name.
	// This can also be used as a cheap way to see if a charm exists without
	// needing to load the charm metadata.
	GetCharmID(ctx context.Context, args charm.GetCharmArgs) (corecharm.ID, error)

	// GetCharm returns the charm metadata for the given charm ID.
	// It returns an error.CharmNotFound if the charm can not be found by the
	// ID.
	GetCharm(ctx context.Context, id corecharm.ID) (internalcharm.Charm, charm.CharmLocator, error)
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
	// We don't currently export applications, that'll be done in a future.
	// For now we need to ensure that we write the charms on the applications.

	for _, app := range model.Applications() {
		// For every application, ensure that the charm is written to the model.
		// This will still be required in the future, it'll just be done in
		// one step.

		metadata := app.CharmMetadata()
		if metadata != nil {
			// The application already has a charm, nothing to do.
			continue
		}

		// TODO(stickupkid): To locate a charm, we currently need to know the
		// charm URL of the application. This is not going to work like this in
		// the future, we can use the charm_uuid instead.

		curl, err := internalcharm.ParseURL(app.CharmURL())
		if err != nil {
			return errors.Errorf("cannot parse charm URL %q: %v", app.CharmURL(), err)
		}

		charmID, err := e.service.GetCharmID(ctx, charm.GetCharmArgs{
			Name:     curl.Name,
			Revision: &curl.Revision,
		})
		if err != nil {
			return errors.Errorf("cannot get charm ID for %q: %v", app.CharmURL(), err)
		}

		charm, _, err := e.service.GetCharm(ctx, charmID)
		if err != nil {
			return errors.Errorf("cannot get charm %q: %v", charmID, err)
		}

		if err := e.exportCharm(ctx, app, charm); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}

func (e *exportOperation) exportCharm(ctx context.Context, app description.Application, charm internalcharm.Charm) error {
	var lxdProfile string
	if profiler, ok := charm.(internalcharm.LXDProfiler); ok {
		var err error
		if lxdProfile, err = e.exportLXDProfile(profiler.LXDProfile()); err != nil {
			return errors.Errorf("cannot export LXD profile: %v", err)
		}
	}

	metadata, err := e.exportCharmMetadata(charm.Meta(), lxdProfile)
	if err != nil {
		return errors.Errorf("cannot export charm metadata: %v", err)
	}

	manifest, err := e.exportCharmManifest(charm.Manifest())
	if err != nil {
		return errors.Errorf("cannot export charm manifest: %v", err)
	}

	config, err := e.exportCharmConfig(charm.Config())
	if err != nil {
		return errors.Errorf("cannot export charm config: %v", err)
	}

	actions, err := e.exportCharmActions(charm.Actions())
	if err != nil {
		return errors.Errorf("cannot export charm actions: %v", err)
	}

	app.SetCharmMetadata(metadata)
	app.SetCharmManifest(manifest)
	app.SetCharmConfigs(config)
	app.SetCharmActions(actions)

	return nil
}

func (e *exportOperation) exportCharmMetadata(metadata *internalcharm.Meta, lxdProfile string) (description.CharmMetadataArgs, error) {
	if metadata == nil {
		return description.CharmMetadataArgs{}, nil
	}

	// Assumes is a recursive structure, so we need to marshal it to JSON as
	// a string, to prevent YAML from trying to interpret it.
	var assumesBytes []byte
	if expr := metadata.Assumes; expr != nil {
		var err error
		assumesBytes, err = json.Marshal(expr)
		if err != nil {
			return description.CharmMetadataArgs{}, errors.Errorf("cannot marshal assumes: %v", err)
		}
	}

	runAs, err := exportCharmUser(metadata.CharmUser)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	provides, err := exportRelations(metadata.Provides)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	requires, err := exportRelations(metadata.Requires)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	peers, err := exportRelations(metadata.Peers)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	extraBindings := exportExtraBindings(metadata.ExtraBindings)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	storage, err := exportStorage(metadata.Storage)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	devices, err := exportDevices(metadata.Devices)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	payloads, err := exportPayloads(metadata.PayloadClasses)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	containers, err := exportContainers(metadata.Containers)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	resources, err := exportResources(metadata.Resources)
	if err != nil {
		return description.CharmMetadataArgs{}, errors.Capture(err)
	}

	return description.CharmMetadataArgs{
		Name:           metadata.Name,
		Summary:        metadata.Summary,
		Description:    metadata.Description,
		Subordinate:    metadata.Subordinate,
		Categories:     metadata.Categories,
		Tags:           metadata.Tags,
		Terms:          metadata.Terms,
		RunAs:          runAs,
		Assumes:        string(assumesBytes),
		MinJujuVersion: metadata.MinJujuVersion.String(),
		Provides:       provides,
		Requires:       requires,
		Peers:          peers,
		ExtraBindings:  extraBindings,
		Storage:        storage,
		Devices:        devices,
		Payloads:       payloads,
		Containers:     containers,
		Resources:      resources,
		LXDProfile:     lxdProfile,
	}, nil
}

func (e *exportOperation) exportLXDProfile(profile *internalcharm.LXDProfile) (string, error) {
	if profile == nil {
		return "", nil
	}

	// The LXD profile is encoded in the description package as a JSON blob.
	// This ensures consistency and prevents accidental encoding issues with
	// YAML.
	data, err := json.Marshal(profile)
	if err != nil {
		return "", errors.Capture(err)
	}

	return string(data), nil
}

func (e *exportOperation) exportCharmManifest(manifest *internalcharm.Manifest) (description.CharmManifestArgs, error) {
	if manifest == nil {
		return description.CharmManifestArgs{}, nil
	}

	bases, err := exportManifestBases(manifest.Bases)
	if err != nil {
		return description.CharmManifestArgs{}, errors.Capture(err)
	}

	return description.CharmManifestArgs{
		Bases: bases,
	}, nil
}

func (e *exportOperation) exportCharmConfig(config *internalcharm.Config) (description.CharmConfigsArgs, error) {
	if config == nil {
		return description.CharmConfigsArgs{}, nil
	}

	configs := make(map[string]description.CharmConfig, len(config.Options))
	for name, option := range config.Options {
		configs[name] = configType{
			typ:          option.Type,
			description:  option.Description,
			defaultValue: option.Default,
		}
	}

	return description.CharmConfigsArgs{
		Configs: configs,
	}, nil
}

func (e *exportOperation) exportCharmActions(actions *internalcharm.Actions) (description.CharmActionsArgs, error) {
	if actions == nil {
		return description.CharmActionsArgs{}, nil
	}

	result := make(map[string]description.CharmAction, len(actions.ActionSpecs))
	for name, action := range actions.ActionSpecs {
		result[name] = actionType{
			description:    action.Description,
			parallel:       action.Parallel,
			executionGroup: action.ExecutionGroup,
			parameters:     action.Params,
		}
	}

	return description.CharmActionsArgs{
		Actions: result,
	}, nil
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

func exportPayloads(payloads map[string]internalcharm.PayloadClass) (map[string]description.CharmMetadataPayload, error) {
	result := make(map[string]description.CharmMetadataPayload, len(payloads))
	for name, payload := range payloads {
		result[name] = payloadType{
			name: payload.Name,
			typ:  payload.Type,
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

type payloadType struct {
	name string
	typ  string
}

// Name returns the name of the payload.
func (p payloadType) Name() string {
	return p.name
}

// Type returns the type of the payload.
func (p payloadType) Type() string {
	return p.typ
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
