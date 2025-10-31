// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"time"

	"github.com/juju/juju/core/application"
	"github.com/juju/juju/core/life"
	corerelation "github.com/juju/juju/core/relation"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/core/unit"
	domainlife "github.com/juju/juju/domain/life"
	domainrelation "github.com/juju/juju/domain/relation"
	"github.com/juju/juju/internal/charm"
)

// TODO (manadart 2025-07-08): entityUUID (type agnostic) should be used in
// place of the typed UUIDs. All other usages of typed identities/names should
// be replaced with strings. This is the database layer, which to the greatest
// extent possible should be aware only of simple types.

// entityUUID is a container for a unique identifier.
type entityUUID struct {
	UUID string `db:"uuid"`
}

type relationUUID struct {
	UUID string `db:"uuid"`
}

type applicationUUID struct {
	UUID string `db:"application_uuid"`
}

type names []string

type name struct {
	Name string `db:"name"`
}

type relation struct {
	UUID    corerelation.UUID `db:"uuid"`
	ID      uint64            `db:"relation_id"`
	LifeID  domainlife.Life   `db:"life_id"`
	ScopeID uint8             `db:"scope_id"`
}

type relationIDAndUUID struct {
	// UUID is the UUID of the relation.
	UUID corerelation.UUID `db:"uuid"`
	// ID is the numeric ID of the relation
	ID uint64 `db:"relation_id"`
}

type relationIDUUIDAppName struct {
	// UUID is the UUID of the relation.
	UUID string `db:"uuid"`
	// ID is the numeric ID of the relation
	ID int `db:"relation_id"`
	// AppName is the name of the application
	AppName string `db:"application_name"`
}

type relationUUIDAndRole struct {
	// UUID is the unique identifier of the relation.
	UUID string `db:"relation_uuid"`
	// Role is the name of the endpoints role, e.g. provider/requirer/peer.
	Role string `db:"role"`
}

type lifeAndSuspended struct {
	Life      life.Value `db:"value"`
	Suspended bool       `db:"suspended"`
	Reason    string     `db:"suspended_reason"`
}

// applicationPlatform represents a structure to get OS and channel information
// for a specific application, from the table `application_platform`
type applicationPlatform struct {
	OS      string `db:"os"`
	Channel string `db:"channel"`
}

type relationUnit struct {
	RelationUnitUUID     string `db:"uuid"`
	RelationEndpointUUID string `db:"relation_endpoint_uuid"`
	RelationUUID         string `db:"relation_uuid"`
	UnitUUID             string `db:"unit_uuid"`
}

// relationUnitWithUnit maps a unit to a relation unit and
// includes the unit name.
type relationUnitWithUnit struct {
	RelationUnitUUID string    `db:"uuid"`
	UnitUUID         unit.UUID `db:"unit_uuid"`
	UnitName         unit.Name `db:"unit_name"`
}

type getUnit struct {
	UUID unit.UUID `db:"uuid"`
	Name unit.Name `db:"name"`
}

type relationUnitUUIDAndName struct {
	RelationUnitUUID corerelation.UnitUUID `db:"uuid"`
	UnitName         unit.Name             `db:"name"`
}

type getRelationUnit struct {
	RelationUUID     corerelation.UUID     `db:"relation_uuid"`
	RelationUnitUUID corerelation.UnitUUID `db:"relation_unit_uuid"`
	UnitUUID         unit.UUID             `db:"unit_uuid"`
	Name             unit.Name             `db:"name"`
}

type getLife struct {
	UUID string     `db:"uuid"`
	Life life.Value `db:"value"`
}

type getUnitApp struct {
	ApplicationUUID string `db:"application_uuid"`
	UnitUUID        string `db:"uuid"`
}

type getUnitRelAndApp struct {
	ApplicationUUID  string `db:"application_uuid"`
	RelationUnitUUID string `db:"uuid"`
	RelationUUID     string `db:"relation_uuid"`
}

type scope struct {
	Scope string `db:"scope"`
}

type getSubordinate struct {
	ApplicationUUID string `db:"application_uuid"`
	Subordinate     bool   `db:"subordinate"`
}

// getPrincipal is used to get the principal application of a unit.
type getPrincipal struct {
	UnitUUID        string `db:"unit_uuid"`
	ApplicationUUID string `db:"application_uuid"`
}

type relationAndApplicationUUID struct {
	RelationUUID  string `db:"relation_uuid"`
	ApplicationID string `db:"application_uuid"`
}

type relationSetting struct {
	Key   string `db:"key"`
	Value string `db:"value"`
}

type relationApplicationSetting struct {
	UUID  string `db:"relation_endpoint_uuid"`
	Key   string `db:"key"`
	Value string `db:"value"`
}

type relationUnitSetting struct {
	UUID  string `db:"relation_unit_uuid"`
	Key   string `db:"key"`
	Value string `db:"value"`
}

type relationUnitSettingName struct {
	UnitName string `db:"name"`
	Key      string `db:"key"`
	Value    string `db:"value"`
}

type applicationSettingsHash struct {
	RelationEndpointUUID string `db:"relation_endpoint_uuid"`
	Hash                 string `db:"sha256"`
}

type nameAndHash struct {
	Hash string `db:"sha256"`
	Name string `db:"name"`
}

type unitSettingsHash struct {
	RelationUnitUUID string `db:"relation_unit_uuid"`
	Hash             string `db:"sha256"`
}

type keys []string

type relationEndpointUUID struct {
	UUID string `db:"uuid"`
}

// endpointIdentifier is an identifier for a relation endpoint.
type endpointIdentifier struct {
	// ApplicationName is the name of the application the endpoint belongs to.
	ApplicationName string `db:"application_name"`
	// EndpointName is the name of the endpoint.
	EndpointName string `db:"endpoint_name"`
}

// goalStateData is per relation data to find goal state.
type goalStateData struct {
	EP1ApplicationName string             `db:"ep1_application_name"`
	EP1EndpointName    string             `db:"ep1_endpoint_name"`
	EP1Role            charm.RelationRole `db:"ep1_role"`
	EP2ApplicationName string             `db:"ep2_application_name"`
	EP2EndpointName    string             `db:"ep2_endpoint_name"`
	EP2Role            charm.RelationRole `db:"ep2_role"`
	Status             corestatus.Status  `db:"status"`
	UpdatedAt          time.Time          `db:"updated_at"`
}

func (g goalStateData) convertToGoalStateRelationData() domainrelation.GoalStateRelationData {
	return domainrelation.GoalStateRelationData{
		Status: g.Status,
		Since:  &g.UpdatedAt,
		EndpointIdentifiers: []corerelation.EndpointIdentifier{
			{
				ApplicationName: g.EP1ApplicationName,
				EndpointName:    g.EP1EndpointName,
				Role:            g.EP1Role,
			}, {
				ApplicationName: g.EP2ApplicationName,
				EndpointName:    g.EP2EndpointName,
				Role:            g.EP2Role,
			},
		},
	}
}

// exportEndpoint contains information needed to export a relation endpoint.
type exportEndpoint struct {
	Endpoint
	// RelationEndpointUUID is a unique identifier for the application endpoint
	RelationEndpointUUID string `db:"relation_endpoint_uuid"`
}

// Endpoint is used to fetch an endpoint from the database. Endpoint is a public
// struct to allow for embedding in exportEndpoint.
type Endpoint struct {
	// ApplicationEndpointUUID is a unique identifier for the application
	// endpoint
	ApplicationEndpointUUID corerelation.EndpointUUID `db:"application_endpoint_uuid"`
	// Endpoint name is the name of the endpoint/relation.
	EndpointName string `db:"endpoint_name"`
	// Role is the name of the endpoints role in the relation.
	Role charm.RelationRole `db:"role"`
	// Interface is the name of the interface this endpoint implements.
	Interface string `db:"interface"`
	// Optional marks if this endpoint is required to be in a relation.
	Optional bool `db:"optional"`
	// Capacity defines the maximum number of supported connections to this
	// relation endpoint.
	Capacity int `db:"capacity"`
	// Scope is the name of the endpoints scope.
	Scope charm.RelationScope `db:"scope"`
	// ApplicationName is the name of the application this endpoint belongs to.
	ApplicationName string `db:"application_name"`
	// ApplicationUUID is a unique identifier for the application associated
	// with the endpoint.
	ApplicationUUID application.UUID `db:"application_uuid"`
}

// String returns a formatted string representation combining
// the ApplicationName and EndpointName of the endpoint.
func (e Endpoint) String() string {
	return fmt.Sprintf("%s:%s", e.ApplicationName, e.EndpointName)
}

// toRelationEndpoint converts an endpoint read out of the database to a
// relation.Endpoint.
func (e Endpoint) toRelationEndpoint() domainrelation.Endpoint {
	return domainrelation.Endpoint{
		ApplicationName: e.ApplicationName,
		Relation: charm.Relation{
			Name:      e.EndpointName,
			Role:      e.Role,
			Interface: e.Interface,
			Optional:  e.Optional,
			Limit:     e.Capacity,
			Scope:     e.Scope,
		},
	}
}

// roEndpointIdentifier returns an EndpointIdentifier type for given
// CandidateEndpointIdentifier.
func (e Endpoint) toEndpointIdentifier() corerelation.EndpointIdentifier {
	return corerelation.EndpointIdentifier{
		ApplicationName: e.ApplicationName,
		EndpointName:    e.EndpointName,
	}
}

// setRelationEndpoint represents the mapping to insert a new relation endpoint
// to the table `relation_endpoint`
type setRelationEndpoint struct {
	UUID         corerelation.EndpointUUID `db:"uuid"`
	RelationUUID corerelation.UUID         `db:"relation_uuid"`
	EndpointUUID corerelation.EndpointUUID `db:"endpoint_uuid"`
}

// setRelationStatus represents the structure to insert the status of a relation.
type setRelationStatus struct {
	// RelationUUID is the unique identifier of the relation.
	RelationUUID corerelation.UUID `db:"relation_uuid"`
	// Status indicates the current state of a given relation.
	Status corestatus.Status `db:"status"`
	// UpdatedAt specifies the timestamp of the insertion
	UpdatedAt time.Time `db:"updated_at"`
}

// otherApplicationsForWatcher contains data required by
// WatchLifeSuspendedStatus watchers.
type otherApplicationsForWatcher struct {
	AppID       application.UUID `db:"application_uuid"`
	Subordinate bool             `db:"subordinate"`
}

type watcherMapperData struct {
	RelationUUID string `db:"uuid"`
	AppUUID      string `db:"application_uuid"`
	Life         string `db:"value"`
	Suspended    bool   `db:"suspended"`
}

// applicationUUIDAndName is used to get the UUID and name of an application.
type applicationUUIDAndName struct {
	ID   application.UUID `db:"uuid"`
	Name string           `db:"name"`
}

// rows is used to count the number of rows found.
type rows struct {
	Count int `db:"count"`
}

type unitUUIDNameLife struct {
	UUID string          `db:"uuid"`
	Name string          `db:"name"`
	Life domainlife.Life `db:"life_id"`
}

type relationNetworkEgress struct {
	RelationUUID string `db:"relation_uuid"`
	CIDR         string `db:"cidr"`
}

type relationStatus struct {
	RelationUUID string     `db:"relation_uuid"`
	StatusID     int        `db:"relation_status_type_id"`
	Message      string     `db:"message"`
	Since        *time.Time `db:"updated_at"`
}

type getRelation struct {
	UUID      string     `db:"uuid"`
	ID        int        `db:"relation_id"`
	Life      life.Value `db:"value"`
	Suspended bool       `db:"suspended"`
}

type relationWithDetails struct {
	UUID      string     `db:"uuid"`
	ID        int        `db:"relation_id"`
	Life      life.Value `db:"life"`
	Suspended bool       `db:"suspended"`
}

// endpointWithRelationUUID combines an Endpoint with its Relation UUID, needed
// for returning relation endpoints mapped by relation.
type endpointWithRelationUUID struct {
	Endpoint
	RelationUUID string `db:"relation_uuid"`
}

// countResultWithRelationUUID is used to return counts grouped by relation
// UUID.
type countResultWithRelationUUID struct {
	RelationUUID string `db:"relation_uuid"`
	Count        int    `db:"count"`
}
