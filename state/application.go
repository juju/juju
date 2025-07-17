// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	stderrors "errors"

	"github.com/juju/mgo/v3/txn"
	"github.com/juju/names/v6"
	"github.com/juju/schema"

	"github.com/juju/juju/core/config"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/configschema"
)

// Application represents the state of an application.
type Application struct {
	st  *State
	doc applicationDoc
}

// applicationDoc represents the internal state of an application in MongoDB.
// Note the correspondence with ApplicationInfo in apiserver.
type applicationDoc struct {
	DocID       string `bson:"_id"`
	Name        string `bson:"name"`
	ModelUUID   string `bson:"model-uuid"`
	Subordinate bool   `bson:"subordinate"`
	// CharmURL should be moved to CharmOrigin. Attempting it should
	// be relatively straight forward, but very time consuming.
	// When moving to CharmHub from Juju it should be
	// tackled then.
	CharmURL    *string     `bson:"charmurl"`
	CharmOrigin CharmOrigin `bson:"charm-origin"`
	// CharmModifiedVersion changes will trigger the upgrade-charm hook
	// for units independent of charm url changes.
	CharmModifiedVersion int   `bson:"charmmodifiedversion"`
	ForceCharm           bool  `bson:"forcecharm"`
	Life                 Life  `bson:"life"`
	UnitCount            int   `bson:"unitcount"`
	TxnRevno             int64 `bson:"txn-revno"`

	// CAAS related attributes.
	PasswordHash string `bson:"passwordhash"`

	// Placement is the placement directive that should be used allocating units/pods.
	Placement string `bson:"placement,omitempty"`
	// HasResources is set to false after an application has been removed
	// and any k8s cluster resources have been fully cleaned up.
	// Until then, the application must not be removed from the Juju model.
	HasResources bool `bson:"has-resources,omitempty"`
}

// name returns the application name.
func (a *Application) name() string {
	return a.doc.Name
}

// Tag returns a name identifying the application.
// The returned name will be different from other Tag values returned by any
// other entities from the same state.
func (a *Application) Tag() names.Tag {
	return names.NewApplicationTag(a.name())
}

// SetCharmConfig contains the parameters for Application.SetCharm.
type SetCharmConfig struct {
	// Charm is the new charm to use for the application. New units
	// will be started with this charm, and existing units will be
	// upgraded to use it.
	Charm CharmRefFull

	// CharmOrigin is the data for where the charm comes from.  Eventually
	// Channel should be move there.
	CharmOrigin *CharmOrigin

	// ConfigSettings is the charm config settings to apply when upgrading
	// the charm.
	ConfigSettings charm.Settings

	// ForceUnits forces the upgrade on units in an error state.
	ForceUnits bool

	// ForceBase forces the use of the charm even if it is not one of
	// the charm's supported series.
	ForceBase bool

	// Force forces the overriding of the lxd profile validation even if the
	// profile doesn't validate.
	Force bool

	// PendingResourceIDs is a map of resource names to resource IDs to activate during
	// the upgrade.
	PendingResourceIDs map[string]string

	// StorageConstraints contains the storage constraints to add or update when
	// upgrading the charm.
	//
	// Any existing storage instances for the named stores will be
	// unaffected; the storage constraints will only be used for
	// provisioning new storage instances.
	StorageConstraints map[string]StorageConstraints

	// EndpointBindings is an operator-defined map of endpoint names to
	// space names that should be merged with any existing bindings.
	EndpointBindings map[string]string
}

// SetCharm changes the charm for the application.
func (a *Application) SetCharm(
	cfg SetCharmConfig,
	store objectstore.ObjectStore,
) (err error) {
	return nil
}

// AddUnitParams contains parameters for the Application.AddUnit method.
type AddUnitParams struct {
	// AttachStorage identifies storage instances to attach to the unit.
	AttachStorage []names.StorageTag

	// These attributes are relevant to CAAS models.

	// ProviderId identifies the unit for a given provider.
	ProviderId *string

	// Address is the container address.
	Address *string

	// Ports are the open ports on the container.
	Ports *[]string

	// UnitName is for CAAS models when creating stateful units.
	UnitName *string

	// PasswordHash is only passed for CAAS sidecar units on creation.
	PasswordHash *string

	// We need charm Meta to add the unit storage and we can't retrieve it
	// from the legacy state so we must pass it here.
	CharmMeta *charm.Meta
}

// AddUnit adds a new principal unit to the application.
func (a *Application) AddUnit(
	args AddUnitParams,
) (unit *Unit, err error) {
	return &Unit{}, nil
}

// UpsertCAASUnitParams is passed to UpsertCAASUnit to describe how to create or how to find and
// update an existing unit for sidecar CAAS application.
type UpsertCAASUnitParams struct {
	AddUnitParams

	// OrderedScale is always true. It represents a mapping of OrderedId to Unit ID.
	OrderedScale bool
	// OrderedId is the stable ordinal index of the "pod".
	OrderedId int

	// ObservedAttachedVolumeIDs is the filesystem attachments observed to be attached by the infrastructure,
	// used to map existing attachments.
	ObservedAttachedVolumeIDs []string
}

// UpdateCharmConfig changes a application's charm config settings. Values set
// to nil will be deleted; unknown and invalid values will return an error.
func (a *Application) UpdateCharmConfig(changes charm.Settings) error {
	return nil
}

// UpdateApplicationConfig changes an application's config settings.
// Unknown and invalid values will return an error.
func (a *Application) UpdateApplicationConfig(
	changes config.ConfigAttributes,
	reset []string,
	schema configschema.Fields,
	defaults schema.Defaults,
) error {
	return nil
}

var ErrSubordinateConstraints = stderrors.New("constraints do not apply to subordinate applications")

// SetConstraints replaces the current application constraints.
func (a *Application) SetConstraints(cons constraints.Value) (err error) {
	if a.doc.Subordinate {
		return ErrSubordinateConstraints
	}

	return nil
}

// StorageConstraints returns the storage constraints for the application.
func (a *Application) StorageConstraints() (map[string]StorageConstraints, error) {
	return nil, nil
}

// UnitUpdateProperties holds information used to update
// the state model for the unit.
type UnitUpdateProperties struct {
	ProviderId           *string
	Address              *string
	Ports                *[]string
	UnitName             *string
	AgentStatus          *status.StatusInfo
	UnitStatus           *status.StatusInfo
	CloudContainerStatus *status.StatusInfo
}

// AddUnitOperation is a model operation that will add a unit.
type AddUnitOperation struct {
}

// Build is part of the ModelOperation interface.
func (op *AddUnitOperation) Build(attempt int) ([]txn.Op, error) {
	return nil, nil
}

// Done is part of the ModelOperation interface.
func (op *AddUnitOperation) Done(err error) error {
	return nil
}
