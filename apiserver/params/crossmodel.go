// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"gopkg.in/juju/charm.v6-unstable"
)

// EndpointFilterAttributes is used to filter offers matching the
// specified endpoint criteria.
type EndpointFilterAttributes struct {
	Role      charm.RelationRole `json:"role"`
	Interface string             `json:"interface"`
	Name      string             `json:"name"`
}

// OfferFilters is used to query offers.
// Offers matching any of the filters are returned.
type OfferFilters struct {
	Filters []OfferFilter
}

// OfferFilter is used to query offers.
type OfferFilter struct {
	ModelName              string                     `json:"model-name"`
	OfferName              string                     `json:"offer-name"`
	ApplicationName        string                     `json:"application-name"`
	ApplicationDescription string                     `json:"application-description"`
	ApplicationUser        string                     `json:"application-user"`
	Endpoints              []EndpointFilterAttributes `json:"endpoints"`
	AllowedUserTags        []string                   `json:"allowed-users"`
}

// ApplicationOffer represents an application offering from an external model.
type ApplicationOffer struct {
	OfferURL               string           `json:"offer-url"`
	OfferName              string           `json:"offer-name"`
	ApplicationDescription string           `json:"application-description"`
	Endpoints              []RemoteEndpoint `json:"endpoints"`
}

// ApplicationOfferDetails represents an application offering,
// including details about how it has been deployed.
type ApplicationOfferDetails struct {
	ApplicationOffer
	ApplicationName string `json:"application-name"`
	CharmName       string `json:"charm-name"`
	ConnectedCount  int    `json:"connected-count"`
}

// ListApplicationOffersResults is a result of listing application offers.
type ListApplicationOffersResults struct {
	// Results contains application offers matching each filter.
	Results []ApplicationOfferDetails `json:"results"`
}

// AddApplicationOffers is used when adding offers to a application directory.
type AddApplicationOffers struct {
	Offers []AddApplicationOffer
}

// AddApplicationOffer values are used to create an application offer.
type AddApplicationOffer struct {
	OfferName              string            `json:"offer-name"`
	ApplicationName        string            `json:"application-name"`
	ApplicationDescription string            `json:"application-description"`
	Endpoints              map[string]string `json:"endpoints"`
}

// RemoteEndpoint represents a remote application endpoint.
type RemoteEndpoint struct {
	Name      string              `json:"name"`
	Role      charm.RelationRole  `json:"role"`
	Interface string              `json:"interface"`
	Limit     int                 `json:"limit"`
	Scope     charm.RelationScope `json:"scope"`
}

// FindApplicationOffersResults is a result of finding remote application offers.
type FindApplicationOffersResults struct {
	// Results contains application offers matching each filter.
	Results []ApplicationOffer `json:"results"`
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

// ApplicationURLs is a collection of remote application URLs
type ApplicationURLs struct {
	// ApplicationURLs contains collection of urls for applications that are to be shown.
	ApplicationURLs []string `json:"application-urls,omitempty"`
}

// ConsumeApplicationArg holds the arguments for consuming a remote application.
type ConsumeApplicationArg struct {
	// ApplicationURLs contains collection of urls for applications that are to be shown.
	ApplicationURL string `json:"application-url"`

	// ApplicationAlias is the name of the alias to use for the application name.
	ApplicationAlias string `json:"application-alias,omitempty"`
}

// ConsumeApplicationArgs is a collection of arg for consuming applications.
type ConsumeApplicationArgs struct {
	Args []ConsumeApplicationArg `json:"args,omitempty"`
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
	ApplicationName    string         `json:"application-name"`
	Endpoint           RemoteEndpoint `json:"endpoint"`
	RemoteEndpointName string         `json:"remote-endpoint-name"`
	SourceModelUUID    string         `json:"source-model-uuid"`
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

	// OfferName is the name of the application on the offering side.
	OfferName string `json:"offer-name"`

	// Life is the current lifecycle state of the application.
	Life Life `json:"life"`

	// Status is the current status of the application.
	Status string `json:"status"`

	// ModelUUID is the UUId of the model hosting the application.
	ModelUUID string `json:"model-uuid"`

	// IsConsumerProxy returns the application is created
	// from a registration operation by a consuming model.
	Registered bool `json:"registered"`
}

// GetTokenArgs holds the arguments to a GetTokens API call.
type GetTokenArgs struct {
	Args []GetTokenArg
}

// GetTokenArg holds the model and entity for which we want a token.
type GetTokenArg struct {
	// ModelTag is the tag of the model hosting the entity.
	ModelTag string `json:"model-tag"`

	// Tag is the tag of the entity for which we want the token.
	Tag string `json:"tag"`
}

// RemoteEntityArgs holds the arguments to an API call dealing with remote entities.
type RemoteEntityArgs struct {
	Args []RemoteEntityArg
}

// RemoteEntityArg holds the model, entity and token to be operated on.
type RemoteEntityArg struct {
	// ModelTag is the tag of the model hosting the entity.
	ModelTag string `json:"model-tag"`

	// Tag is the tag of the entity.
	Tag string `json:"tag"`

	// Token is the token of the entity.
	Token string `json:"token,omitempty"`
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

	// TODO(wallyworld) - status etc
}

// RemoteApplicationChanges describes a set of changes to remote
// applications.
type RemoteApplicationChanges struct {
	Changes []RemoteApplicationChange `json:"changes,omitempty"`
}

// RemoteRelationsChanges holds a set of RemoteRelationsChange structures.
type RemoteRelationsChanges struct {
	Changes []RemoteRelationChangeEvent `json:"changes,omitempty"`
}

// RemoteRelationUnitChange describes a relation unit change
// which has occurred in a remote model.
type RemoteRelationUnitChange struct {
	// UnitId uniquely identifies the remote unit.
	UnitId int `json:"unit-id"`

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

	// ApplicationId is the application id on the remote model.
	ApplicationId RemoteEntityId `json:"application-id"`

	// ChangedUnits maps unit tokens to relation unit changes.
	ChangedUnits []RemoteRelationUnitChange `json:"changed-units,omitempty"`

	// DepartedUnits contains the ids of units that have departed
	// the relation since the last change.
	DepartedUnits []int `json:"departed-units,omitempty"`
}

// RegisterRemoteRelation holds attributes used to register a remote relation.
type RegisterRemoteRelation struct {
	// ApplicationId is the application id on the remote model.
	ApplicationId RemoteEntityId `json:"application-id"`

	// RelationId is the relation id on the remote model.
	RelationId RemoteEntityId `json:"relation-id"`

	// RemoteEndpoint contains info about the endpoint in the remote model.
	RemoteEndpoint RemoteEndpoint `json:"remote-endpoint"`

	// OfferName is the name of the application offer from the local model.
	OfferName string `json:"offer-name"`

	// LocalEndpointName is the name of the endpoint in the local model.
	LocalEndpointName string `json:"local-endpoint-name"`
}

// RegisterRemoteRelations holds args used to add remote relations.
type RegisterRemoteRelations struct {
	Relations []RegisterRemoteRelation `json:"relations"`
}

// RemoteApplicationInfo has attributes for a remote application.
type RemoteApplicationInfo struct {
	ModelTag       string `json:"model-tag"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ApplicationURL string `json:"application-url"`
	// SourceModelLabel is only populated if the application
	// originates from another model on the same controller
	// rather than via an offer URL.
	SourceModelLabel string           `json:"source-model-label,omitempty"`
	Endpoints        []RemoteEndpoint `json:"endpoints"`
	// IconURLPath is relative to the model api endpoint
	IconURLPath string `json:"icon-url-path"`
}

// RemoteApplicationInfoResult holds the result of loading
// remote application info at a URL.
type RemoteApplicationInfoResult struct {
	Result *RemoteApplicationInfo `json:"result,omitempty"`
	Error  *Error                 `json:"error,omitempty"`
}

// RemoteApplicationInfoResults represents the result of a RemoteApplicationInfo call.
type RemoteApplicationInfoResults struct {
	Results []RemoteApplicationInfoResult `json:"results"`
}

// ConsumeApplicationResult is the response for one request to consume
// a remote application.
type ConsumeApplicationResult struct {
	LocalName string `json:"local-name,omitempty"`
	Error     *Error `json:"error,omitempty"`
}

// ConsumeApplicationResults is the result of a Consume call.
type ConsumeApplicationResults struct {
	Results []ConsumeApplicationResult `json:"results"`
}

// RemoteEntities identifies multiple remote entities.
type RemoteEntities struct {
	Entities []RemoteEntityId `json:"remote-entities"`
}

// IngressSubnetInfo is the result of an IngressSubnetsForRelation call.
type IngressSubnetInfo struct {
	// CIDRs is the set if CIDRs which need to be allowed ingress to the application.
	CIDRs []string `json:"cidrs,omitempty"`
}

// IngressSubnetResult holds ingress network information and an error.
type IngressSubnetResult struct {
	Error  *Error             `json:"error,omitempty"`
	Result *IngressSubnetInfo `json:"result,omitempty"`
}

// IngressSubnetResults holds the result of an API call that returns
// information about ingress networks for multiple remote relations.
type IngressSubnetResults struct {
	Results []IngressSubnetResult `json:"results"`
}
