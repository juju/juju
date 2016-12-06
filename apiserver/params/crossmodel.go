// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// OfferFilterParams contains filters used to query application offers
// from one or more directories.
type OfferFilterParams struct {
	Filters []OfferFilters `json:"filters"`
}

// EndpointFilterAttributes is used to filter offers matching the
// specified endpoint criteria.
type EndpointFilterAttributes struct {
	Role      charm.RelationRole `json:"role"`
	Interface string             `json:"interface"`
	Name      string             `json:"name"`
}

// OfferFilters is used to query offers in an application directory.
// Offers matching any of the filters are returned.
type OfferFilters struct {
	Directory string
	Filters   []OfferFilter
}

// OfferFilter is used to query offers in a application directory.
type OfferFilter struct {
	ApplicationURL         string                     `json:"application-url"`
	SourceLabel            string                     `json:"source-label"`
	SourceModelUUIDTag     string                     `json:"source-model-uuid"`
	ApplicationName        string                     `json:"application-name"`
	ApplicationDescription string                     `json:"application-description"`
	ApplicationUser        string                     `json:"application-user"`
	Endpoints              []EndpointFilterAttributes `json:"endpoints"`
	AllowedUserTags        []string                   `json:"allowed-users"`
}

// ApplicationOffer represents an application offering from an external model.
type ApplicationOffer struct {
	ApplicationURL         string           `json:"application-url"`
	SourceModelTag         string           `json:"source-model-tag"`
	SourceLabel            string           `json:"source-label"`
	ApplicationName        string           `json:"application-name"`
	ApplicationDescription string           `json:"application-description"`
	Endpoints              []RemoteEndpoint `json:"endpoints"`
}

// AddApplicationOffers is used when adding offers to a application directory.
type AddApplicationOffers struct {
	Offers []AddApplicationOffer
}

// AddApplicationOffer represents a application offering from an external environment.
type AddApplicationOffer struct {
	ApplicationOffer
	// UserTags are those who can consume the offer.
	UserTags []string `json:"users"`
}

// ApplicationOfferResults is a result of listing application offers.
type ApplicationOfferResults struct {
	Offers []ApplicationOffer
	Error  *Error
}

// RemoteEndpoint represents a remote application endpoint.
type RemoteEndpoint struct {
	Name      string              `json:"name"`
	Role      charm.RelationRole  `json:"role"`
	Interface string              `json:"interface"`
	Limit     int                 `json:"limit"`
	Scope     charm.RelationScope `json:"scope"`
}

// ApplicationOfferParams is used to offer remote applications.
type ApplicationOfferParams struct {
	// ModelTag is the tag of the model containing the application to offer.
	ModelTag string `json:"model-tag"`

	// ApplicationURL may contain user supplied application url.
	ApplicationURL string `json:"application-url,omitempty"`

	// ApplicationName contains name of application being offered.
	ApplicationName string `json:"application-name"`

	// ApplicationDescription is description for the offered application.
	// For now, this defaults to description provided in the charm or
	// is supplied by the user.
	ApplicationDescription string `json:"application-description"`

	// Endpoints contains offered application endpoints.
	Endpoints []string `json:"endpoints"`

	// AllowedUserTags contains tags of users that are allowed to use this offered application.
	AllowedUserTags []string `json:"allowed-users"`
}

// ApplicationOffersParams contains a collection of offers to allow adding offers in bulk.
type ApplicationOffersParams struct {
	Offers []ApplicationOfferParams `json:"offers"`
}

// FindApplicationOffersResults is a result of finding remote application offers.
type FindApplicationOffersResults struct {
	// Results contains application offers matching each filter.
	Results []ApplicationOfferResults `json:"results"`
}

// ApplicationOfferResult is a result of listing a remote application offer.
type ApplicationOfferResult struct {
	// Result contains application offer information.
	Result ApplicationOffer `json:"result"`

	// Error contains related error.
	Error *Error `json:"error,omitempty"`
}

// ApplicationOffersResults is a result of listing remote application offers.
type ApplicationOffersResults struct {
	// Result contains collection of remote application results.
	Results []ApplicationOfferResult `json:"results,omitempty"`
}

// ApplicationURLs is a filter used to select remote applications via show call.
type ApplicationURLs struct {
	// ApplicationURLs contains collection of urls for applications that are to be shown.
	ApplicationURLs []string `json:"application-urls,omitempty"`
}

// OfferedApplication represents attributes for an offered application.
type OfferedApplication struct {
	ApplicationURL  string            `json:"application-url"`
	ApplicationName string            `json:"application-name"`
	CharmName       string            `json:"charm-name"`
	Description     string            `json:"description"`
	Registered      bool              `json:"registered"`
	Endpoints       map[string]string `json:"endpoints"`
}

// OfferedApplicationResult holds the result of loading an
// offered application at a URL.
type OfferedApplicationResult struct {
	Result OfferedApplication `json:"result"`
	Error  *Error             `json:"error,omitempty"`
}

// OfferedApplicationResults represents the result of a ListOfferedApplications call.
type OfferedApplicationResults struct {
	Results []OfferedApplicationResult
}

// OfferedApplicationDetails is an application found during a request to list remote applications.
type OfferedApplicationDetails struct {
	// ApplicationURL may contain user supplied application URL.
	ApplicationURL string `json:"application-url,omitempty"`

	// ApplicationName contains name of application being offered.
	ApplicationName string `json:"application-name"`

	// CharmName is the charm name of this application.
	CharmName string `json:"charm-name"`

	// UsersCount is the count of how many users are connected to this shared application.
	UsersCount int `json:"users-count,omitempty"`

	// Endpoints is a list of charm relations that this remote application offered.
	Endpoints []RemoteEndpoint `json:"endpoints"`
}

// OfferedApplicationDetailsResult is a result of listing a remote application.
type OfferedApplicationDetailsResult struct {
	// Result contains remote application information.
	Result *OfferedApplicationDetails `json:"result,omitempty"`

	// Error contains error related to this item.
	Error *Error `json:"error,omitempty"`
}

// ListOffersFilterResults is a result of listing remote application offers
// for an application directory.
type ListOffersFilterResults struct {
	// Error contains error related to this directory.
	Error *Error `json:"error,omitempty"`

	// Result contains collection of remote application item results for this directory.
	Result []OfferedApplicationDetailsResult `json:"result,omitempty"`
}

// ListOffersResults is a result of listing remote application offers
// for application directories.
type ListOffersResults struct {
	// Results contains collection of remote directories results.
	Results []ListOffersFilterResults `json:"results,omitempty"`
}

// OfferedApplicationFilters has sets of filters that
// are used by a vendor to query remote applications that the vendor has offered.
type OfferedApplicationFilters struct {
	Filters []OfferedApplicationFilter `json:"filters,omitempty"`
}

// OfferedApplicationFilter has a set of filter terms that
// are used by a vendor to query remote applications that the vendor has offered.
type OfferedApplicationFilter struct {
	FilterTerms []OfferedApplicationFilterTerm `json:"filter-terms,omitempty"`
}

// OfferedApplicationFilterTerm has filter criteria that
// are used by a vendor to query remote applications that the vendor has offered.
type OfferedApplicationFilterTerm struct {
	// ApplicationURL is url for remote application.
	// This may be a part of valid URL.
	ApplicationURL string `json:"application-url,omitempty"`

	// Endpoint contains endpoint properties for filter.
	Endpoint RemoteEndpoint `json:"endpoint"`

	// CharmName is the charm name of this application.
	CharmName string `json:"charm-name,omitempty"`
}

// RemoteEntityId is an identifier for an entity that may be involved in a
// cross-model relation. This object comprises the UUID of the model to
// which the entity belongs, and an opaque token that is unique to that model.
type RemoteEntityId struct {
	ModelUUID string `json:"model-uuid"`
	Token     string `json:"token"`
}

// RemoteEntityIdResult holds a remote entity id and an error.
type RemoteEntityIdResult struct {
	Result *RemoteEntityId `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// RemoteEntityIdResults has a set of remote entity id results.
type RemoteEntityIdResults struct {
	Results []RemoteEntityIdResult `json:"results,omitempty"`
}

// RemoteRelation describes the current state of a cross-model relation from
// the perspective of the local model.
type RemoteRelation struct {
	Life               Life           `json:"life"`
	Id                 int            `json:"id"`
	Key                string         `json:"key"`
	LocalEndpoint      RemoteEndpoint `json:"local-endpoint"`
	RemoteEndpointName string         `json:"remote-endpoint-name"`
}

// RemoteRelationResult holds a remote relation and an error.
type RemoteRelationResult struct {
	Error  *Error          `json:"error,omitempty"`
	Result *RemoteRelation `json:"result,omitempty"`
}

// RemoteRelationResults holds the result of an API call that returns
// information about multiple remote relations.
type RemoteRelationResults struct {
	Results []RemoteRelationResult `json:"results"`
}

// RemoteApplication describes the current state of an application involved in a cross-
// model relation, from the perspective of the local environment.
type RemoteApplication struct {
	// Name is the name of the application.
	Name string `json:"name"`

	// Life is the current lifecycle state of the application.
	Life Life `json:"life"`

	// Status is the current status of the application.
	Status string `json:"status"`

	// ModelUUID is the UUId of the model hosting the application.
	ModelUUID string `json:"model-uuid"`

	// Registered returns the application is created
	// from a registration operation by a consuming model.
	Registered bool `json:"registered"`
}

// RemoteApplicationResult holds a remote application and an error.
type RemoteApplicationResult struct {
	Result *RemoteApplication `json:"result,omitempty"`
	Error  *Error             `json:"error,omitempty"`
}

// RemoteApplicationResults holds a set of remote application results.
type RemoteApplicationResults struct {
	Results []RemoteApplicationResult `json:"results,omitempty"`
}

// RemoteRelationsWatchResult holds a RemoteRelationsWatcher id,
// changes and an error (if any).
type RemoteRelationsWatchResult struct {
	RemoteRelationsWatcherId string
	Change                   *RemoteRelationsChange `json:"change,omitempty"`
	Error                    *Error                 `json:"error,omitempty"`
}

// RemoteRelationsWatchResults holds the results for any API call which ends
// up returning a list of RemoteRelationsWatchers.
type RemoteRelationsWatchResults struct {
	Results []RemoteRelationsWatchResult `json:"results"`
}

// RemoteApplicationWatchResult holds a RemoteApplicationWatcher id,
// changes and an error (if any).
type RemoteApplicationWatchResult struct {
	RemoteApplicationWatcherId string                   `json:"id"`
	Change                     *RemoteApplicationChange `json:"change,omitempty"`
	Error                      *Error                   `json:"error,omitempty"`
}

// RemoteApplicationWatchResults holds the results for any API call which ends
// up returning a list of RemoteServiceWatchers.
type RemoteApplicationWatchResults struct {
	Results []RemoteApplicationWatchResult `json:"results,omitempty"`
}

// RemoteApplicationChange describes changes to an application.
type RemoteApplicationChange struct {
	// ApplicationTag is the tag of the application.
	ApplicationTag string `json:"application-tag"`

	// Life is the current lifecycle state of the application.
	Life Life `json:"life"`

	// Relations are the relations which have changed.
	Relations RemoteRelationsChange `json:"relations"`
	// TODO(wallyworld) - status etc
}

// RemoteApplicationChanges describes a set of changes to remote
// applications.
type RemoteApplicationChanges struct {
	Changes []RemoteApplicationChange `json:"changes,omitempty"`
}

// RemoteRelationsChange describes changes to the relations that a remote
// service is involved in.
type RemoteRelationsChange struct {
	// Initial indicates whether or not this is an initial, complete
	// representation of all relations involving a service. If Initial
	// is true, then RemovedRelations will be empty.
	Initial bool `json:"initial"`

	// ChangedRelations contains relation changes.
	ChangedRelations []RemoteRelationChange `json:"changed,omitempty"`

	// RemovedRelations contains the IDs corresponding to
	// relations removed since the last change.
	RemovedRelations []int `json:"removed,omitempty"`
}

// RemoteRelationsChanges holds a set of RemoteRelationsChange structures.
type RemoteRelationsChanges struct {
	Changes []RemoteRelationChangeEvent `json:"changes,omitempty"`
}

// RemoteRelationChange describes changes to a relation involving
// a remote application.
type RemoteRelationChange struct {
	// RelationId is the numeric ID of the relation.
	RelationId int `json:"id"`

	// Life is the current lifecycle state of the relation.
	Life Life `json:"life"`

	// ChangedUnits maps unit names to relation unit changes.
	ChangedUnits map[string]RemoteRelationUnitChange `json:"changed-units,omitempty"`

	// DepartedUnits contains the names of units that have departed
	// the relation since the last change.
	DepartedUnits []string `json:"departed-units,omitempty"`
}

// RemoteRelationUnitChange describes a relation unit change
// which has occurred in a remote model.
type RemoteRelationUnitChange struct {
	// UnitId uniquely identifies the remote unit.
	UnitId RemoteEntityId `json:"unit-id"`

	// Settings is the current settings for the relation unit.
	Settings map[string]interface{} `json:"settings,omitempty"`
}

// RemoteRelationChangeEvent is pushed to the remote model to communicate
// changes to relation units from the local model.
type RemoteRelationChangeEvent struct {
	// RelationId is the remote id of the relation.
	RelationId RemoteEntityId `json:"relation-id"`

	// Life is the current lifecycle state of the relation.
	Life Life `json:"life"`

	// ChangedUnits maps unit tokens to relation unit changes.
	ChangedUnits []RemoteRelationUnitChange `json:"changed-units,omitempty"`

	// DepartedUnits contains the remote ids of units that have departed
	// the relation since the last change.
	DepartedUnits []RemoteEntityId `json:"departed-units,omitempty"`
}

// RegisterRemoteRelation holds attributes used to register a remote relation.
type RegisterRemoteRelation struct {
	// ApplicationId is the application id on the remote model.
	ApplicationId RemoteEntityId `json:"application-id"`

	// RelationId is the relation id on the remote model.
	RelationId RemoteEntityId `json:"relation-id"`

	// RemoteEndpoint contains info about the endpoint in the remote model.
	RemoteEndpoint RemoteEndpoint `json:"remote-endpoint"`

	// OfferedApplicationName is the name of the application offer from the local model.
	OfferedApplicationName string `json:"offered-application-name"`

	// LocalEndpointName is the name of the endpoint in the local model.
	LocalEndpointName string `json:"local-endpoint-name"`
}

// RegisterRemoteRelations holds args used to add remote relations.
type RegisterRemoteRelations struct {
	Relations []RegisterRemoteRelation `json:"relations"`
}
