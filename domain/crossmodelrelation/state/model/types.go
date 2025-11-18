// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"maps"
	"slices"
	"time"

	corelife "github.com/juju/juju/core/life"
	coresecrets "github.com/juju/juju/core/secrets"
	corestatus "github.com/juju/juju/core/status"
	"github.com/juju/juju/domain/application/architecture"
	"github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/domain/crossmodelrelation"
	"github.com/juju/juju/domain/life"
	domainsecret "github.com/juju/juju/domain/secret"
)

type uuids []string

// nameAndUUID is an agnostic container for the pair of
// `uuid` and `name` columns.
type nameAndUUID struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

// offerEndpoint represent a row in the offer_endpoint table.
type offerEndpoint struct {
	OfferUUID    string `db:"offer_uuid"`
	EndpointUUID string `db:"endpoint_uuid"`
}

// uuid is an agnostic container for a `uuid` column.
type uuid struct {
	UUID string `db:"uuid"`
}

type applicationUUIDAndCharmSource struct {
	UUID        string            `db:"uuid"`
	CharmSource charm.CharmSource `db:"name"`
}

// name is an agnostic container for a `name` column.
type name struct {
	Name string `db:"name"`
}

type offerAndApplicationUUID struct {
	UUID            string `db:"uuid"`
	ApplicationUUID string `db:"application_uuid"`
}

type consumeDetail struct {
	OfferUUID         string             `db:"uuid"`
	EndpointName      string             `db:"name"`
	EndpointRole      charm.RelationRole `db:"role"`
	EndpointInterface string             `db:"interface"`
	EndpointLimit     int                `db:"capacity"`
}

// offerDetail contains the data necessary for create
// OfferDetail structures
type offerDetail struct {
	OfferUUID              string `db:"offer_uuid"`
	OfferName              string `db:"offer_name"`
	ApplicationName        string `db:"application_name"`
	ApplicationDescription string `db:"application_description"`
	TotalConnections       int    `db:"total_connections"`
	TotalActiveConnections int    `db:"total_active_connections"`

	// CharmLocator parts
	CharmName         string                    `db:"charm_name"`
	CharmRevision     int                       `db:"charm_revision"`
	CharmSource       charm.CharmSource         `db:"charm_source"`
	CharmArchitecture architecture.Architecture `db:"charm_architecture"`

	// OfferEndpoint parts
	EndpointName      string             `db:"endpoint_name"`
	EndpointRole      charm.RelationRole `db:"endpoint_role"`
	EndpointInterface string             `db:"endpoint_interface"`
}

type offerFilter struct {
	OfferName              string `db:"offer_name"`
	ApplicationName        string `db:"application_name"`
	ApplicationDescription string `db:"application_description"`
	EndpointName           string `db:"endpoint_name"`
	Interface              string `db:"endpoint_interface"`
	Role                   string `db:"endpoint_role"`
}

type offerDetails []offerDetail

func (o offerDetails) TransformToOfferDetails() []*crossmodelrelation.OfferDetail {
	converted := make(map[string]*crossmodelrelation.OfferDetail, 0)
	for _, details := range o {
		found, ok := converted[details.OfferUUID]
		if ok {
			// TODO  - ensure endpoints unique
			// Seen this offer before, add more endpoints, and keep going.
			found.Endpoints = append(found.Endpoints, crossmodelrelation.OfferEndpoint{
				Name:      details.EndpointName,
				Role:      details.EndpointRole,
				Interface: details.EndpointInterface,
			})
			converted[details.OfferUUID] = found
			continue
		}
		// New offer, add with one endpoint.
		found = &crossmodelrelation.OfferDetail{
			OfferUUID:              details.OfferUUID,
			OfferName:              details.OfferName,
			ApplicationName:        details.ApplicationName,
			ApplicationDescription: details.ApplicationDescription,
			CharmLocator: charm.CharmLocator{
				Name:         details.CharmName,
				Revision:     details.CharmRevision,
				Source:       details.CharmSource,
				Architecture: details.CharmArchitecture,
			},
			Endpoints: []crossmodelrelation.OfferEndpoint{
				{
					Name:      details.EndpointName,
					Role:      details.EndpointRole,
					Interface: details.EndpointInterface,
				},
			},
			TotalConnections:       details.TotalConnections,
			TotalActiveConnections: details.TotalActiveConnections,
		}
		converted[details.OfferUUID] = found
	}

	return slices.Collect(maps.Values(converted))
}

type setApplicationDetails struct {
	UUID      string    `db:"uuid"`
	Name      string    `db:"name"`
	CharmUUID string    `db:"charm_uuid"`
	LifeID    life.Life `db:"life_id"`
	SpaceUUID string    `db:"space_uuid"`
}

type countResult struct {
	Count int `db:"count"`
}

// setCharmState is used to set the charm.
type setCharmState struct {
	UUID          string `db:"uuid"`
	ReferenceName string `db:"reference_name"`
	SourceID      int    `db:"source_id"`
}

// setCharmMetadata is used to set the metadata of a charm.
// This includes the setting of the LXD profile.
type setCharmMetadata struct {
	CharmUUID   string `db:"charm_uuid"`
	Name        string `db:"name"`
	Description string `db:"description"`
}

// setCharmRelation is used to set the relations of a charm.
type setCharmRelation struct {
	UUID      string `db:"uuid"`
	CharmUUID string `db:"charm_uuid"`
	Name      string `db:"name"`
	RoleID    int    `db:"role_id"`
	Interface string `db:"interface"`
	Capacity  int    `db:"capacity"`
	ScopeID   int    `db:"scope_id"`
}

type remoteApplicationOfferer struct {
	// UUID is the unique identifier for this remote application offerer.
	UUID string `db:"uuid"`
	// LifeID is the life state of the remote application offerer.
	LifeID life.Life `db:"life_id"`
	// ApplicationUUID is the unique identifier for the application
	// that is being offered.
	ApplicationUUID string `db:"application_uuid"`
	// OfferUUID is the offer uuid that ties both the offerer and consumer
	// together.
	OfferUUID string `db:"offer_uuid"`
	// OfferURL is the URL of the offer that the remote application is
	// consuming.
	OfferURL string `db:"offer_url"`
	// OffererControllerUUID is the unique identifier for the controller
	// that is offering this application.
	OffererControllerUUID sql.Null[string] `db:"offerer_controller_uuid"`
	// OffererModelUUID is the unique identifier for the model
	// that is offering this application.
	OffererModelUUID string `db:"offerer_model_uuid"`
	// Macaroon is the serialized macaroon that can be used to
	// authenticate to the offerer controller.
	Macaroon []byte `db:"macaroon"`
}

type remoteApplicationOffererInfo struct {
	// LifeID is the life state of the remote application offerer.
	LifeID life.Life `db:"life_id"`
	// ApplicationUUID is the unique identifier for the application
	// that is being offered.
	ApplicationUUID string `db:"application_uuid"`
	// ApplicationName is the name of the application that is being offered.
	ApplicationName string `db:"application_name"`
	// OfferUUID is the offer uuid that ties both the offerer and consumer
	// together.
	OfferUUID string `db:"offer_uuid"`
	// OfferURL is the URL of the offer that the remote application is
	// consuming.
	OfferURL string `db:"offer_url"`
	// OffererControllerUUID is the unique identifier for the controller
	// that is offering this application.
	OffererControllerUUID sql.Null[string] `db:"offerer_controller_uuid"`
	// OffererModelUUID is the unique identifier for the model
	// that is offering this application.
	OffererModelUUID string `db:"offerer_model_uuid"`
	// Macaroon is the serialized macaroon that can be used to
	// authenticate to the offerer controller.
	Macaroon []byte `db:"macaroon"`
}

type remoteApplicationStatus struct {
	RemoteApplicationUUID string     `db:"application_remote_offerer_uuid"`
	StatusID              int        `db:"status_id"`
	Message               string     `db:"message"`
	Data                  []byte     `db:"data"`
	UpdatedAt             *time.Time `db:"updated_at"`
}

type remoteApplicationConsumer struct {
	OffererApplicationUUID  string    `db:"offerer_application_uuid"`
	ConsumerApplicationUUID string    `db:"consumer_application_uuid"`
	OfferConnectionUUID     string    `db:"offer_connection_uuid"`
	ConsumerModelUUID       string    `db:"consumer_model_uuid"`
	LifeID                  life.Life `db:"life_id"`
}

type offerConnection struct {
	UUID               string `db:"uuid"`
	OfferUUID          string `db:"offer_uuid"`
	RemoteRelationUUID string `db:"remote_relation_uuid"`
	Username           string `db:"username"`
}

type offerConnectionDetail struct {
	OfferUUID     string    `db:"offer_uuid"`
	EndpointName  string    `db:"name"`
	RelationUUID  string    `db:"relation_uuid"`
	RelationID    uint64    `db:"relation_id"`
	Since         time.Time `db:"updated_at"`
	Status        string    `db:"status"`
	StatusMessage string    `db:"message"`
	Username      string    `db:"username"`
}

type remoteRelationUUID struct {
	UUID string `db:"uuid"`
}

type offerConnectionQuery struct {
	OfferUUID string `db:"offer_uuid"`
}

type relation struct {
	UUID       string `db:"uuid"`
	LifeID     int    `db:"life_id"`
	RelationID uint64 `db:"relation_id"`
}

type relationEndpoint struct {
	UUID            string `db:"uuid"`
	RelationUUID    string `db:"relation_uuid"`
	ApplicationUUID string `db:"application_uuid"`
	EndpointName    string `db:"endpoint_name"`
}

type addRelationEndpointArgs struct {
	RelationUUID       string
	ApplicationOneUUID string
	EndpointOneName    string
	ApplicationTwoUUID string
	EndpointTwoName    string
}

type charmScope struct {
	Name string `db:"name"`
}

type setApplicationEndpointBinding struct {
	UUID          string `db:"uuid"`
	ApplicationID string `db:"application_uuid"`
	RelationUUID  string `db:"charm_relation_uuid"`
}

// charmRelationName represents is used to fetch relation of a charm when only
// the name is required
type charmRelationName struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type modelUUID struct {
	UUID string `db:"uuid"`
}

type applicationUUID struct {
	UUID string `db:"uuid"`
}

type unit struct {
	UUID string `db:"uuid"`
	Name string `db:"name"`
}

type secretRevisions []revisionUUID
type revisionUUID struct {
	UUID string `db:"uuid"`
}

type secretRemoteUnitConsumer struct {
	UnitName        string `db:"unit_name"`
	SecretID        string `db:"secret_id"`
	CurrentRevision int    `db:"current_revision"`
}

type secretRemoteUnitConsumers []secretRemoteUnitConsumer

func (rows secretRemoteUnitConsumers) toSecretConsumers() []*coresecrets.SecretConsumerMetadata {
	result := make([]*coresecrets.SecretConsumerMetadata, len(rows))
	for i, row := range rows {
		result[i] = &coresecrets.SecretConsumerMetadata{
			CurrentRevision: row.CurrentRevision,
		}
	}
	return result
}

type secretRef struct {
	ID         string `db:"secret_id"`
	SourceUUID string `db:"source_uuid"`
}

type secretLatestRevision struct {
	ID              string `db:"secret_id"`
	LatestRevision  int    `db:"latest_revision"`
	ApplicationUUID string `db:"owner_application_uuid"`
}

type secretUnitConsumer struct {
	UnitUUID        string `db:"unit_uuid"`
	SecretID        string `db:"secret_id"`
	SourceModelUUID string `db:"source_model_uuid"`
	Label           string `db:"label"`
	CurrentRevision int    `db:"current_revision"`
}

type secretRevisionObsolete struct {
	ID            string `db:"revision_uuid"`
	Obsolete      bool   `db:"obsolete"`
	PendingDelete bool   `db:"pending_delete"`
}

type unitName struct {
	Name string `db:"name"`
}
type unitNames []string

type unitRow struct {
	ApplicationID string    `db:"application_uuid"`
	UnitUUID      string    `db:"uuid"`
	CharmUUID     string    `db:"charm_uuid"`
	Name          string    `db:"name"`
	NetNodeID     string    `db:"net_node_uuid"`
	LifeID        life.Life `db:"life_id"`
}

type remoteApplicationConsumerInfo struct {
	ApplicationName string    `db:"application_name"`
	LifeID          life.Life `db:"life_id"`
	OfferUUID       string    `db:"offer_uuid"`
}

type secretAccessor struct {
	SecretID      string                        `db:"secret_id"`
	SubjectID     string                        `db:"subject_id"`
	SubjectTypeID domainsecret.GrantSubjectType `db:"subject_type_id"`
	RoleID        domainsecret.Role             `db:"role_id"`
}

type secretRole struct {
	Role string `db:"role"`
}

type secretValueRef struct {
	BackendUUID string `db:"backend_uuid"`
	RevisionID  string `db:"revision_id"`
}

type secretContent struct {
	Name    string `db:"name"`
	Content string `db:"content"`
}

type secretRevision struct {
	SecretID string `db:"secret_id"`
	Revision int    `db:"revision"`
}

type secretValues []secretContent

func (rows secretValues) toSecretData() coresecrets.SecretData {
	result := make(coresecrets.SecretData)
	for _, row := range rows {
		result[row.Name] = row.Content
	}
	return result
}

type relationSuspended struct {
	IsSuspended bool `db:"suspended"`
}

type endpointIdentifier struct {
	// ApplicationName is the name of the application the endpoint belongs to.
	ApplicationName string `db:"application_name"`
	// EndpointName is the name of the endpoint.
	EndpointName string `db:"endpoint_name"`
}

type relationNetworkIngress struct {
	RelationUUID string `db:"relation_uuid"`
	CIDR         string `db:"cidr"`
}

type cidr struct {
	CIDR string `db:"cidr"`
}

type lifeID struct {
	Life int `db:"life_id"`
}

type queryLife struct {
	Value corelife.Value `db:"value"`
}

type modelConfigKey struct {
	Key string `db:"key"`
}

type modelConfigValue struct {
	Value string `db:"value"`
}

type unitAddress struct {
	UnitUUID   string         `db:"unit_uuid"`
	Value      string         `db:"address_value"`
	ConfigType string         `db:"config_type_name"`
	Type       string         `db:"type_name"`
	Origin     string         `db:"origin_name"`
	Scope      string         `db:"scope_name"`
	SpaceUUID  sql.NullString `db:"space_uuid"`
	CIDR       sql.NullString `db:"cidr"`
}

// setRelationStatus represents the structure to insert the status of a relation.
type setRelationStatus struct {
	// RelationUUID is the unique identifier of the relation.
	RelationUUID string `db:"relation_uuid"`
	// Status indicates the current state of a given relation.
	Status corestatus.Status `db:"status"`
	// UpdatedAt specifies the timestamp of the insertion
	UpdatedAt time.Time `db:"updated_at"`
}

// remoteModelUUID is used to fetch the UUID of a remote model. It can be NULL
// in the case of the relation not being CMR.
type remoteModelUUID struct {
	UUID sql.NullString `db:"uuid"`
}
