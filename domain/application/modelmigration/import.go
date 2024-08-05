// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelmigration

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/juju/description/v8"
	"github.com/juju/errors"
	"github.com/juju/version/v2"

	coreapplication "github.com/juju/juju/core/application"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/modelmigration"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/application/service"
	"github.com/juju/juju/domain/application/state"
	"github.com/juju/juju/internal/charm"
	internalcharm "github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/charm/assumes"
	"github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/storage"
)

// Coordinator is the interface that is used to add operations to a migration.
type Coordinator interface {
	Add(modelmigration.Operation)
}

// RegisterImport register's a new model migration importer into the supplied
// coordinator.
func RegisterImport(coordinator Coordinator, registry storage.ProviderRegistry, logger logger.Logger) {
	coordinator.Add(&importOperation{
		registry:     registry,
		logger:       logger,
		charmOrigins: make(map[string]*corecharm.Origin),
	})
}

type importOperation struct {
	modelmigration.BaseOperation

	logger logger.Logger

	service  ImportService
	registry storage.ProviderRegistry

	charmOrigins map[string]*corecharm.Origin
}

// ImportService defines the application service used to import applications
// from another controller model to this controller.
type ImportService interface {
	// CreateApplication registers the existence of an application in the model.
	CreateApplication(context.Context, string, internalcharm.Charm, corecharm.Origin, service.AddApplicationArgs, ...service.AddUnitArg) (coreapplication.ID, error)
}

// Name returns the name of this operation.
func (i *importOperation) Name() string {
	return "import applications"
}

func (i *importOperation) Setup(scope modelmigration.Scope) error {
	i.service = service.NewService(
		state.NewState(scope.ModelDB(), i.logger),
		i.registry,
		i.logger,
	)
	return nil
}

func ptr[T any](v T) *T {
	return &v
}

func (i *importOperation) Execute(ctx context.Context, model description.Model) error {
	for _, app := range model.Applications() {
		unitArgs := make([]service.AddUnitArg, 0, len(app.Units()))
		for _, unit := range app.Units() {
			arg := service.AddUnitArg{
				UnitName: ptr(unit.Name()),
			}
			if unit.PasswordHash() != "" {
				arg.PasswordHash = ptr(unit.PasswordHash())
			}
			if cc := unit.CloudContainer(); cc != nil {
				cldContainer := &service.CloudContainerParams{}
				cldContainer.Address, cldContainer.AddressOrigin = i.makeAddress(cc.Address())
				if cc.ProviderId() != "" {
					cldContainer.ProviderId = ptr(cc.ProviderId())
				}
				if len(cc.Ports()) > 0 {
					cldContainer.Ports = ptr(cc.Ports())
				}
				arg.CloudContainer = cldContainer
			}
			unitArgs = append(unitArgs, arg)
		}

		charm, err := i.importCharm(ctx, charmData{
			Metadata: app.CharmMetadata(),
			Manifest: app.CharmManifest(),
			Actions:  app.CharmActions(),
			Config:   app.CharmConfigs(),
		})
		if err != nil {
			return fmt.Errorf("import model application %q charm: %w", app.Name(), err)
		}

		origin, err := i.importCharmOrigin(app)
		if err != nil {
			return fmt.Errorf("parse charm origin %v: %w", app.CharmOrigin(), err)
		}

		_, err = i.service.CreateApplication(
			ctx, app.Name(), charm, *origin, service.AddApplicationArgs{}, unitArgs...,
		)
		if err != nil {
			return fmt.Errorf(
				"import model application %q with %d units: %w",
				app.Name(), len(app.Units()), err,
			)
		}
	}

	return nil
}

// importCharmOrigin returns the charm origin for an application
//
// Ensure ID, Hash and Channel are dropped from local charm.
// Due to LP:1986547: where the track is missing from the effective channel it implicitly
// resolves to 'latest' if the charm does not have a default channel defined. So if the
// received channel has no track, we can be confident it should be 'latest'
func (i *importOperation) importCharmOrigin(a description.Application) (*corecharm.Origin, error) {
	sourceOrigin := a.CharmOrigin()
	if sourceOrigin == nil {
		return nil, errors.Errorf("nil charm origin importing application %q", a.Name())
	}
	_, err := internalcharm.ParseURL(a.CharmURL())
	if err != nil {
		return nil, errors.Trace(err)
	}

	if foundOrigin, ok := i.charmOrigins[a.CharmURL()]; ok {
		return foundOrigin, nil
	}

	var channel *internalcharm.Channel
	serialized := sourceOrigin.Channel()
	if serialized != "" && corecharm.CharmHub.Matches(sourceOrigin.Source()) {
		c, err := internalcharm.ParseChannelNormalize(serialized)
		if err != nil {
			return nil, errors.Trace(err)
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
		return nil, errors.Trace(err)
	}
	platform := corecharm.Platform{
		Architecture: p.Architecture,
		OS:           p.OS,
		Channel:      p.Channel,
	}

	rev := sourceOrigin.Revision()
	// We can hardcode type to charm as we never store bundles in state.
	var origin *corecharm.Origin
	if corecharm.Local.Matches(sourceOrigin.Source()) {
		origin = &corecharm.Origin{
			Source:   corecharm.Local,
			Type:     "charm",
			Revision: &rev,
			Platform: platform,
		}
	} else if corecharm.CharmHub.Matches(sourceOrigin.Source()) {
		origin = &corecharm.Origin{
			Source:   corecharm.CharmHub,
			Type:     "charm",
			Revision: &rev,
			Platform: platform,
			ID:       sourceOrigin.ID(),
			Hash:     sourceOrigin.Hash(),
			Channel:  channel,
		}
	} else {
		return nil, errors.Errorf("unrecognised charm origin %q", sourceOrigin.Source())
	}

	i.charmOrigins[a.CharmURL()] = origin
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

type stubCharm struct {
	name     string
	revision int
}

type charmData struct {
	Metadata description.CharmMetadata
	Manifest description.CharmManifest
	Actions  description.CharmActions
	Config   description.CharmConfigs
}

// Import the application charm description from the migrating model into
// the current model. This will then be saved with the application and allow
// us to keep RI of the application and the charm.
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
		return nil, fmt.Errorf("import charm metadata: %w", err)
	}

	manifest, err := i.importCharmManifest(data.Manifest)
	if err != nil {
		return nil, fmt.Errorf("import charm manifest: %w", err)
	}

	lxdProfile, err := i.importCharmLXDProfile(data.Metadata)
	if err != nil {
		return nil, fmt.Errorf("import charm lxd profile: %w", err)
	}

	// Return a valid charm base that can then be used to create the
	// application.
	return internalcharm.NewCharmBase(metadata, manifest, nil, nil, lxdProfile), nil
}

func (i *importOperation) importCharmMetadata(data description.CharmMetadata) (*internalcharm.Meta, error) {
	var (
		err    error
		runsAs internalcharm.RunAs
	)
	if runsAs, err = importCharmUser(data); err != nil {
		return nil, fmt.Errorf("import charm user: %w", err)
	}

	var assumes *assumes.ExpressionTree
	if assumes, err = importAssumes(data.Assumes()); err != nil {
		return nil, fmt.Errorf("import charm assumes: %w", err)
	}

	var minJujuVersion version.Number
	if minJujuVersion, err = importMinJujuVersion(data.MinJujuVersion()); err != nil {
		return nil, fmt.Errorf("import min juju version: %w", err)
	}

	var provides map[string]internalcharm.Relation
	if provides, err = importRelations(data.Provides()); err != nil {
		return nil, fmt.Errorf("import provides relations: %w", err)
	}

	var requires map[string]internalcharm.Relation
	if requires, err = importRelations(data.Requires()); err != nil {
		return nil, fmt.Errorf("import requires relations: %w", err)
	}

	var peers map[string]internalcharm.Relation
	if peers, err = importRelations(data.Peers()); err != nil {
		return nil, fmt.Errorf("import peers relations: %w", err)
	}

	var extraBindings map[string]internalcharm.ExtraBinding
	if extraBindings, err = importExtraBindings(data.ExtraBindings()); err != nil {
		return nil, fmt.Errorf("import extra bindings: %w", err)
	}

	var storage map[string]internalcharm.Storage
	if storage, err = importStorage(data.Storage()); err != nil {
		return nil, fmt.Errorf("import storage: %w", err)
	}

	var devices map[string]internalcharm.Device
	if devices, err = importDevices(data.Devices()); err != nil {
		return nil, fmt.Errorf("import devices: %w", err)
	}

	var payloadClasses map[string]internalcharm.PayloadClass
	if payloadClasses, err = importPayloadClasses(data.Payloads()); err != nil {
		return nil, fmt.Errorf("import payload classes: %w", err)
	}

	var containers map[string]internalcharm.Container
	if containers, err = importContainers(data.Containers()); err != nil {
		return nil, fmt.Errorf("import containers: %w", err)
	}

	var resources map[string]resource.Meta
	if resources, err = importResources(data.Resources()); err != nil {
		return nil, fmt.Errorf("import resources: %w", err)
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
		ExtraBindings:  extraBindings,
		Storage:        storage,
		Devices:        devices,
		PayloadClasses: payloadClasses,
		Containers:     containers,
		Resources:      resources,
	}, nil
}

func (i *importOperation) importCharmManifest(data description.CharmManifest) (*internalcharm.Manifest, error) {
	bases, err := importManifestBases(data.Bases())
	if err != nil {
		return nil, fmt.Errorf("import manifest bases: %w", err)
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
		return nil, fmt.Errorf("unmarshal lxd profile: %w", err)
	}

	return &profile, nil
}

func importCharmUser(data description.CharmMetadata) (internalcharm.RunAs, error) {
	switch data.RunAs() {
	case "default", "":
		return internalcharm.RunAsDefault, nil
	case "root":
		return internalcharm.RunAsRoot, nil
	case "sudoer":
		return internalcharm.RunAsSudoer, nil
	case "non-root":
		return internalcharm.RunAsNonRoot, nil
	default:
		return internalcharm.RunAsDefault, fmt.Errorf("unknown run-as value %q", data.RunAs())
	}
}

func importAssumes(data string) (*assumes.ExpressionTree, error) {
	// Assumes i§§s a recursive structure, rather than sending all that data over
	// the wire as yaml, the description package encodes that information as
	// a JSON string.

	// If the data is empty, we don't have any assumes at all, so it's safe
	// to just return nil.
	if data == "" {
		return nil, nil
	}

	var tree *assumes.ExpressionTree
	if err := tree.UnmarshalJSON([]byte(data)); err != nil {
		return nil, fmt.Errorf("unmarshal assumes: %w", err)
	}
	return tree, nil
}

func importMinJujuVersion(data string) (version.Number, error) {
	// minJujuVersion is optional, so if the data is empty, we can just return
	// an empty version.
	if data == "" {
		return version.Number{}, nil
	}

	ver, err := version.Parse(data)
	if err != nil {
		return version.Number{}, fmt.Errorf("parse min juju version: %w", err)
	}
	return ver, nil
}

func importRelations(data map[string]description.CharmMetadataRelation) (map[string]internalcharm.Relation, error) {
	relations := make(map[string]internalcharm.Relation, len(data))
	for name, rel := range data {
		role, err := importRelationRole(rel.Role())
		if err != nil {
			return nil, fmt.Errorf("import relation role: %w", err)
		}

		scope, err := importRelationScope(rel.Scope())
		if err != nil {
			return nil, fmt.Errorf("import relation scope: %w", err)
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
	case "peer":
		return internalcharm.RolePeer, nil
	case "provider":
		return internalcharm.RoleProvider, nil
	case "requirer":
		return internalcharm.RoleRequirer, nil
	default:
		return "", fmt.Errorf("unknown relation role %q", data)
	}
}

func importRelationScope(data string) (internalcharm.RelationScope, error) {
	switch data {
	case "global":
		return internalcharm.ScopeGlobal, nil
	case "container":
		return internalcharm.ScopeContainer, nil
	default:
		return "", fmt.Errorf("unknown relation scope %q", data)
	}
}

func importExtraBindings(data map[string]string) (map[string]internalcharm.ExtraBinding, error) {
	extraBindings := make(map[string]internalcharm.ExtraBinding, len(data))
	for key, name := range data {
		extraBindings[key] = internalcharm.ExtraBinding{
			Name: name,
		}
	}
	return extraBindings, nil
}

func importStorage(data map[string]description.CharmMetadataStorage) (map[string]internalcharm.Storage, error) {
	storage := make(map[string]internalcharm.Storage, len(data))
	for name, s := range data {
		typ, err := importStorageType(s.Type())
		if err != nil {
			return nil, fmt.Errorf("import storage type: %w", err)
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
	case "block":
		return internalcharm.StorageBlock, nil
	case "filesystem":
		return internalcharm.StorageFilesystem, nil
	default:
		return "", fmt.Errorf("unknown storage type %q", data)
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

func importPayloadClasses(data map[string]description.CharmMetadataPayload) (map[string]internalcharm.PayloadClass, error) {
	payloadClasses := make(map[string]internalcharm.PayloadClass, len(data))
	for name, p := range data {
		payloadClasses[name] = internalcharm.PayloadClass{
			Name: p.Name(),
			Type: p.Type(),
		}
	}
	return payloadClasses, nil
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
			return nil, fmt.Errorf("import resource type: %w", err)
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
	case "file":
		return resource.TypeFile, nil
	case "oci-image":
		return resource.TypeContainerImage, nil
	default:
		return -1, fmt.Errorf("unknown resource type %q", data)
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
			return nil, fmt.Errorf("import channel for %q: %w", base.Name(), err)
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
	return charm.ParseChannel(data)
}
