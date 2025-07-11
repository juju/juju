// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/devices"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/resource"
	"github.com/juju/juju/core/status"
	coreunit "github.com/juju/juju/core/unit"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	apperrors "github.com/juju/juju/domain/application/errors"
	domainstorage "github.com/juju/juju/domain/storage"
	internalcharm "github.com/juju/juju/internal/charm"
	charmresource "github.com/juju/juju/internal/charm/resource"
	"github.com/juju/juju/internal/errors"
)

// AddApplicationArgs contains arguments for adding an application to the model.
type AddApplicationArgs struct {
	// ReferenceName is the given name of the charm that is stored in the
	// persistent storage. The proxy name should either be the application
	// name or the charm metadata name.
	//
	// The name of a charm can differ from the charm name stored in the metadata
	// in the cases where the application name is selected by the user.
	// In order to select that charm again via the name, we need to use the
	// proxy name to locate it. You can't go via the application and select it
	// via the application name, as no application might be referencing it at
	// that specific revision. The only way to then locate the charm directly
	// via the name is use the proxy name.
	ReferenceName string

	// CharmStoragePath is the path to the charm in the storage.
	CharmStoragePath string

	// CharmObjectStoreUUID is the UUID of the object store where the charm is
	// stored.
	CharmObjectStoreUUID objectstore.UUID

	// StorageDirectiveOverrides defines a set of storage directive overrides
	// for the application as a map of storage name to overrides. Each name in
	// the map must match a storage name in the charm. Any other value will be
	// an error.
	StorageDirectiveOverrides map[string]ApplicationStorageDirectiveOverride

	// DownloadInfo contains the download information for the charm.
	DownloadInfo *domaincharm.DownloadInfo

	// ResolvedResources contains a list of ResolvedResource instances,
	// which allows to define a revision and an origin for each resource.
	ResolvedResources ResolvedResources

	// PendingResources are the uuids of resources added before the
	// application is created.
	PendingResources []resource.UUID

	// ApplicationConfig contains the application config.
	ApplicationConfig config.ConfigAttributes

	// ApplicationSettings contains the application settings.
	ApplicationSettings application.ApplicationSettings

	// ApplicationStatus contains the application status. It's optional
	// and if not provided, the application will be started with no status.
	ApplicationStatus *status.StatusInfo

	// Constraints contains the application constraints.
	Constraints constraints.Value

	// EndpointBindings is a map to bind application endpoint by name to a
	// specific space. The default space is referenced by an empty key, if any.
	EndpointBindings map[string]network.SpaceName

	// Devices contains the device constraints for the application.
	Devices map[string]devices.Constraints

	// Placement is the placement of the application units.
	Placement *instance.Placement

	// IsController indicates whether the application is a controller
	// application. This should only be set to true if the application
	// is the controller application for the controller model.
	IsController bool
}

// ApplicationStorageDirectiveOverride defines a single set of overrides for a
// storage directive to accompany an application when it is being added.
// Typically, this is supplied by the caller when the user whishes to set
// explicitly storage directive parameters of the application.
//
// Each value in this struct is optional and only when a value that is non nil
// has been supplied will it be used to override the defaults.
type ApplicationStorageDirectiveOverride struct {
	// Count is the number of storage instances to create for each unit. This
	// value must be greater or equal to the minimum defined by the charm. This
	// value must also be less or equal to the maximum defined by the charm.
	Count *uint32

	// ProviderType defines the type of the provider to use when provisioning
	// storage for this directive. Only
	ProviderType *string

	// PoolUUID defines the storage pool to use when provisioning storage for
	// this directive. Either this value or
	// [AddApplicationStorageDirectiveArg.ProviderType] can be set. If both are
	// defined this is an error.
	PoolUUID *domainstorage.StoragePoolUUID

	// Size defines the size of the storage to provision as a minimum value in
	// MiB. What gets provisioned by the provider for each unit may be larger
	// then this value.
	Size *uint64
}

// AddressParams contains parameters for a unit/cloud container address.
type AddressParams struct {
	Value       string
	AddressType string
	Scope       string
	Origin      string
	SpaceID     string
}

// AddUnitArg contains parameters for adding a unit to the model.
type AddUnitArg struct {
	Placement *instance.Placement

	// Storage params go here.
}

// AddIAASUnitArg contains parameters for adding a IAAS unit to the model.
type AddIAASUnitArg struct {
	AddUnitArg
	Nonce *string
}

// ImportUnitArg contains parameters for inserting a fully
// populated unit into the model, eg during migration.
type ImportUnitArg struct {
	UnitName       coreunit.Name
	PasswordHash   *string
	CloudContainer *application.CloudContainerParams
	Machine        machine.Name
	// Principal contains the name of the units principal unit. If the unit is
	// not a subordinate, this field is empty.
	Principal coreunit.Name
}

// UpdateCAASUnitParams contains parameters for updating a CAAS unit.
type UpdateCAASUnitParams struct {
	ProviderID           *string
	Address              *string
	Ports                *[]string
	AgentStatus          *status.StatusInfo
	WorkloadStatus       *status.StatusInfo
	CloudContainerStatus *status.StatusInfo
}

// ScalingState contains attributes that describes
// the scaling state of a CAAS application.
type ScalingState struct {
	ScaleTarget int
	Scaling     bool
}

// ResolvedResources is a collection of ResolvedResource elements.
type ResolvedResources []ResolvedResource

// ResolvedResource represents a resource with a given name, origin, and optional revision.
type ResolvedResource struct {
	Name     string
	Origin   charmresource.Origin
	Revision *int
}

// Validate checks the ResolvedResource's attributes for validity and returns an error if invalid.
// Returns a [apperrors.InvalidResourceArgs] if:
// - the resource name is empty,
// - the resource origin is not valid,
// - the revision is not defined for a resource originated from store
// - the revision is defined for a resource originated from upload
func (r ResolvedResource) Validate() error {
	if r.Name == "" {
		return errors.Errorf("resource name is empty: %w", apperrors.InvalidResourceArgs)
	}
	if err := r.Origin.Validate(); err != nil {
		return errors.Errorf("resource origin %q is invalid: %w", r.Origin,
			apperrors.InvalidResourceArgs)
	}
	if r.Origin == charmresource.OriginUpload && r.Revision != nil {
		return errors.Errorf("resource revision should be nil with %q origin: %w", r.Origin,
			apperrors.InvalidResourceArgs)
	}
	return nil
}

// Validate checks the validity of each ResolvedResource in the collection.
// It accumulates errors and returns a combined error, if any invalid resources are encountered.
func (r ResolvedResources) Validate() error {
	var errs []error
	for _, res := range r {
		if err := res.Validate(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ImportApplicationArgs contains arguments for importing an application to the
// model.
type ImportApplicationArgs struct {
	// Charm is the charm to import.
	Charm internalcharm.Charm

	// CharmOrigin is the origin of the charm.
	CharmOrigin corecharm.Origin

	// ReferenceName is the given name of the charm that is stored in the
	// persistent storage. The proxy name should either be the application
	// name or the charm metadata name.
	//
	// The name of a charm can differ from the charm name stored in the metadata
	// in the cases where the application name is selected by the user.
	// In order to select that charm again via the name, we need to use the
	// proxy name to locate it. You can't go via the application and select it
	// via the application name, as no application might be referencing it at
	// that specific revision. The only way to then locate the charm directly
	// via the name is use the proxy name.
	ReferenceName string

	// ApplicationConfig contains the application config.
	ApplicationConfig config.ConfigAttributes

	// ApplicationSettings contains the application settings.
	ApplicationSettings application.ApplicationSettings

	// ResolvedResources contains a list of ResolvedResource instances,
	// TODO (stickupkid): This isn't currently wired up.
	ResolvedResources ResolvedResources

	// Units contains the units to import.
	Units []ImportUnitArg

	// ApplicationConstraints contains the application constraints.
	ApplicationConstraints constraints.Value

	// CharmUpgradeOnError indicates whether the charm must be upgraded
	// even when on error.
	CharmUpgradeOnError bool

	// ScaleState is the scale state (including scaling, scale and scale
	// target) of the application.
	ScaleState application.ScaleState

	// EndpointBindings are the endpoint bindings for the charm
	EndpointBindings map[string]network.SpaceName

	// ExposedEndpoints is the exposed endpoints for the application.
	ExposedEndpoints map[string]application.ExposedEndpoint

	// PeerRelations is a map of peer relation endpoint to relation id.
	PeerRelations map[string]int
}

// ApplicationConfig represents the application config for the specified
// application ID.
type ApplicationConfig struct {
	CharmOrigin       corecharm.Origin
	CharmConfig       internalcharm.Config
	ApplicationConfig config.ConfigAttributes
	Trust             bool
	CharmName         string
	Principal         bool
}
