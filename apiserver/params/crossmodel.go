// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"gopkg.in/juju/charm.v6"
	"gopkg.in/macaroon.v2-unstable"
)

// ExternalControllerInfoResults contains the results of querying
// the information for a set of external controllers.
type ExternalControllerInfoResults struct {
	Results []ExternalControllerInfoResult `json:"results"`
}

// ExternalControllerInfoResult contains the result of querying
// the information of external controllers.
type ExternalControllerInfoResult struct {
	Result *ExternalControllerInfo `json:"result"`
	Error  *Error                  `json:"error"`
}

// SetControllersInfoParams contains the parameters for setting the
// info for a set of external controllers.
type SetExternalControllersInfoParams struct {
	Controllers []SetExternalControllerInfoParams `json:"controllers"`
}

// SetExternalControllerInfoParams contains the parameters for setting
// the info for an external controller.
type SetExternalControllerInfoParams struct {
	Info ExternalControllerInfo `json:"info"`
}

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
	// OwnerName is the owner of the model hosting the offer.
	OwnerName string `json:"owner-name"`

	// ModelName is the name of the model hosting the offer.
	ModelName string `json:"model-name"`

	OfferName              string                     `json:"offer-name"`
	ApplicationName        string                     `json:"application-name"`
	ApplicationDescription string                     `json:"application-description"`
	ApplicationUser        string                     `json:"application-user"`
	Endpoints              []EndpointFilterAttributes `json:"endpoints"`
	ConnectedUserTags      []string                   `json:"connected-users"`
	AllowedConsumerTags    []string                   `json:"allowed-users"`
}

// ApplicationOfferDetails represents an application offering from an external model.
type ApplicationOfferDetails struct {
	SourceModelTag         string             `json:"source-model-tag"`
	OfferUUID              string             `json:"offer-uuid"`
	OfferURL               string             `json:"offer-url"`
	OfferName              string             `json:"offer-name"`
	ApplicationDescription string             `json:"application-description"`
	Endpoints              []RemoteEndpoint   `json:"endpoints,omitempty"`
	Spaces                 []RemoteSpace      `json:"spaces,omitempty"`
	Bindings               map[string]string  `json:"bindings,omitempty"`
	Users                  []OfferUserDetails `json:"users,omitempty"`
}

// OfferUserDetails represents an offer consumer and their permission on the offer.
type OfferUserDetails struct {
	UserName    string `json:"user"`
	DisplayName string `json:"display-name"`
	Access      string `json:"access"`
}

// ApplicationOfferAdminDetails represents an application offering,
// including details about how it has been deployed.
type ApplicationOfferAdminDetails struct {
	ApplicationOfferDetails
	ApplicationName string            `json:"application-name"`
	CharmURL        string            `json:"charm-url"`
	Connections     []OfferConnection `json:"connections,omitempty"`
}

// OfferConnection holds details about a connection to an offer.
type OfferConnection struct {
	SourceModelTag string       `json:"source-model-tag"`
	RelationId     int          `json:"relation-id"`
	Username       string       `json:"username"`
	Endpoint       string       `json:"endpoint"`
	Status         EntityStatus `json:"status"`
	IngressSubnets []string     `json:"ingress-subnets"`
}

// QueryApplicationOffersResults is a result of searching application offers.
type QueryApplicationOffersResults struct {
	// Results contains application offers matching each filter.
	Results []ApplicationOfferAdminDetails `json:"results"`
}

// AddApplicationOffers is used when adding offers to a application directory.
type AddApplicationOffers struct {
	Offers []AddApplicationOffer
}

// AddApplicationOffer values are used to create an application offer.
type AddApplicationOffer struct {
	ModelTag               string            `json:"model-tag"`
	OfferName              string            `json:"offer-name"`
	ApplicationName        string            `json:"application-name"`
	ApplicationDescription string            `json:"application-description"`
	Endpoints              map[string]string `json:"endpoints"`
}

// DestroyApplicationOffers holds parameters for the DestroyOffers call.
type DestroyApplicationOffers struct {
	OfferURLs []string `json:"offer-urls"`
	Force     bool     `json:"force,omitempty"`
}

// RemoteEndpoint represents a remote application endpoint.
type RemoteEndpoint struct {
	Name      string             `json:"name"`
	Role      charm.RelationRole `json:"role"`
	Interface string             `json:"interface"`
	Limit     int                `json:"limit"`
}

// RemoteSpace represents a space in some remote model.
type RemoteSpace struct {
	CloudType          string                 `json:"cloud-type"`
	Name               string                 `json:"name"`
	ProviderId         string                 `json:"provider-id"`
	ProviderAttributes map[string]interface{} `json:"provider-attributes"`
	Subnets            []Subnet               `json:"subnets"`
}

// ApplicationOfferResult is a result of querying a remote
// application offer based on its URL.
type ApplicationOfferResult struct {
	// Result contains application offer information.
	Result *ApplicationOfferAdminDetails `json:"result,omitempty"`

	// Error contains related error.
	Error *Error `json:"error,omitempty"`
}

// ApplicationOffersResults is a result of listing remote application offers.
type ApplicationOffersResults struct {
	// Results contains collection of remote application results.
	Results []ApplicationOfferResult `json:"results,omitempty"`
}

// OfferURLs is a collection of remote offer URLs
type OfferURLs struct {
	// OfferURLs contains collection of urls for applications that are to be shown.
	OfferURLs []string `json:"offer-urls,omitempty"`
}

// ConsumeApplicationArg holds the arguments for consuming a remote application.
type ConsumeApplicationArg struct {
	// The offer to be consumed.
	ApplicationOfferDetails

	// Macaroon is used for authentication.
	Macaroon *macaroon.Macaroon `json:"macaroon,omitempty"`

	// ControllerInfo contains connection details to the controller
	// hosting the offer.
	ControllerInfo *ExternalControllerInfo `json:"external-controller,omitempty"`

	// ApplicationAlias is the name of the alias to use for the application name.
	ApplicationAlias string `json:"application-alias,omitempty"`
}

// ConsumeApplicationArgs is a collection of arg for consuming applications.
type ConsumeApplicationArgs struct {
	Args []ConsumeApplicationArg `json:"args,omitempty"`
}

// TokenResult holds a token and an error.
type TokenResult struct {
	Token string `json:"token,omitempty"`
	Error *Error `json:"error,omitempty"`
}

// TokenResults has a set of token results.
type TokenResults struct {
	Results []TokenResult `json:"results,omitempty"`
}

// RemoteRelation describes the current state of a cross-model relation from
// the perspective of the local model.
type RemoteRelation struct {
	Life                  Life           `json:"life"`
	Suspended             bool           `json:"suspended"`
	Id                    int            `json:"id"`
	Key                   string         `json:"key"`
	ApplicationName       string         `json:"application-name"`
	Endpoint              RemoteEndpoint `json:"endpoint"`
	RemoteApplicationName string         `json:"remote-application-name"`
	RemoteEndpointName    string         `json:"remote-endpoint-name"`
	SourceModelUUID       string         `json:"source-model-uuid"`
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
// model relation, from the perspective of the local model.
type RemoteApplication struct {
	// Name is the name of the application.
	Name string `json:"name"`

	// OfferUUID is the uuid of the application offer.
	OfferUUID string `json:"offer-uuid"`

	// Life is the current lifecycle state of the application.
	Life Life `json:"life,omitempty"`

	// Status is the current status of the application.
	Status string `json:"status,omitempty"`

	// ModelUUID is the UUId of the model hosting the application.
	ModelUUID string `json:"model-uuid"`

	// IsConsumerProxy returns if the application is created
	// from a registration operation by a consuming model.
	IsConsumerProxy bool `json:"is-consumer-proxy"`

	// Macaroon is used for authentication.
	Macaroon *macaroon.Macaroon `json:"macaroon,omitempty"`
}

// GetTokenArgs holds the arguments to a GetTokens API call.
type GetTokenArgs struct {
	Args []GetTokenArg
}

// GetTokenArg holds the model and entity for which we want a token.
type GetTokenArg struct {
	// Tag is the tag of the entity for which we want the token.
	Tag string `json:"tag"`
}

// RemoteEntityTokenArgs holds the arguments to an API call dealing with
// remote entities and their tokens.
type RemoteEntityTokenArgs struct {
	Args []RemoteEntityTokenArg
}

// RemoteEntityTokenArg holds the entity and token to be operated on.
type RemoteEntityTokenArg struct {
	// Tag is the tag of the entity.
	Tag string `json:"tag"`

	// Token is the token of the entity.
	Token string `json:"token,omitempty"`
}

// EntityMacaroonArgs holds the arguments to a SaveMacaroons API call.
type EntityMacaroonArgs struct {
	Args []EntityMacaroonArg
}

// EntityMacaroonArg holds a macaroon and entity which we want to save.
type EntityMacaroonArg struct {
	Macaroon *macaroon.Macaroon `json:"macaroon"`
	Tag      string             `json:"tag"`
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
	// RelationToken is the token of the relation.
	RelationToken string `json:"relation-token"`

	// ApplicationToken is the token of the application.
	ApplicationToken string `json:"application-token"`

	// Life is the current lifecycle state of the relation.
	Life Life `json:"life"`

	// ForceCleanup is true if the offering side should forcibly
	// ensure that all relation units have left scope.
	ForceCleanup *bool `json:"force-cleanup,omitempty"`

	// Suspended is the current suspended status of the relation.
	Suspended *bool `json:"suspended,omitempty"`

	SuspendedReason string `json:"suspended-reason,omitempty"`

	// ChangedUnits maps unit tokens to relation unit changes.
	ChangedUnits []RemoteRelationUnitChange `json:"changed-units,omitempty"`

	// DepartedUnits contains the ids of units that have departed
	// the relation since the last change.
	DepartedUnits []int `json:"departed-units,omitempty"`

	// Macaroons are used for authentication.
	Macaroons macaroon.Slice `json:"macaroons,omitempty"`
}

// RelationLifeSuspendedStatusChange describes the life
// and suspended status of a relation.
type RelationLifeSuspendedStatusChange struct {
	// Key is the relation key of the changed relation.
	Key string `json:"key"`

	// Life is the life of the relation.
	Life Life `json:"life"`

	// Suspended is the suspended status of the relation.
	Suspended bool `json:"suspended"`

	// SuspendedReason is an optional message to explain why suspended is true.
	SuspendedReason string `json:"suspended-reason"`
}

// RelationLifeSuspendedStatusWatchResult holds a RelationStatusWatcher id, baseline state
// (in the Changes field), and an error (if any).
type RelationLifeSuspendedStatusWatchResult struct {
	RelationStatusWatcherId string                              `json:"watcher-id"`
	Changes                 []RelationLifeSuspendedStatusChange `json:"changes"`
	Error                   *Error                              `json:"error,omitempty"`
}

// RelationStatusWatchResults holds the results for any API call which ends up
// returning a list of RelationStatusWatchers.
type RelationStatusWatchResults struct {
	Results []RelationLifeSuspendedStatusWatchResult `json:"results"`
}

// OfferStatusChange describes the status of an offer.
type OfferStatusChange struct {
	// OfferName is the name of the offer.
	OfferName string `json:"offer-name"`

	// Status is the status of the offer.
	Status EntityStatus `json:"status"`
}

// OfferStatusWatchResult holds a OfferStatusWatcher id, baseline state
// (in the Changes field), and an error (if any).
type OfferStatusWatchResult struct {
	OfferStatusWatcherId string              `json:"watcher-id"`
	Changes              []OfferStatusChange `json:"changes"`
	Error                *Error              `json:"error,omitempty"`
}

// OfferStatusWatchResults holds the results for any API call which ends up
// returning a list of OfferStatusWatchers.
type OfferStatusWatchResults struct {
	Results []OfferStatusWatchResult `json:"results"`
}

// IngressNetworksChanges holds a set of IngressNetworksChangeEvent structures.
type IngressNetworksChanges struct {
	Changes []IngressNetworksChangeEvent `json:"changes,omitempty"`
}

type IngressNetworksChangeEvent struct {
	// RelationToken is the token of the relation.
	RelationToken string `json:"relation-token"`

	// ApplicationToken is the token of the application.
	ApplicationToken string `json:"application-token"`

	// Networks are the CIDRs for which ingress is required.
	Networks []string `json:"networks,omitempty"`

	// IngressRequired is true if ingress is needed, otherwise
	// ingress should be disabled.
	IngressRequired bool `json:"ingress-required"`

	// Macaroons are used for authentication.
	Macaroons macaroon.Slice `json:"macaroons,omitempty"`
}

// RegisterRemoteRelationArg holds attributes used to register a remote relation.
type RegisterRemoteRelationArg struct {
	// ApplicationToken is the application token on the remote model.
	ApplicationToken string `json:"application-token"`

	// SourceModelTag is the tag of the model hosting the application.
	SourceModelTag string `json:"source-model-tag"`

	// RelationToken is the relation token on the remote model.
	RelationToken string `json:"relation-token"`

	// RemoteEndpoint contains info about the endpoint in the remote model.
	RemoteEndpoint RemoteEndpoint `json:"remote-endpoint"`

	// RemoteSpace contains provider-level info about the space the
	// endpoint is bound to in the remote model.
	RemoteSpace RemoteSpace `json:"remote-space"`

	// OfferUUID is the UUID of the offer.
	OfferUUID string `json:"offer-uuid"`

	// LocalEndpointName is the name of the endpoint in the local model.
	LocalEndpointName string `json:"local-endpoint-name"`

	// Macaroons are used for authentication.
	Macaroons macaroon.Slice `json:"macaroons,omitempty"`
}

// RegisterRemoteRelationArgs holds args used to add remote relations.
type RegisterRemoteRelationArgs struct {
	Relations []RegisterRemoteRelationArg `json:"relations"`
}

// RegisterRemoteRelationResult holds a remote relation details and an error.
type RegisterRemoteRelationResult struct {
	Result *RemoteRelationDetails `json:"result,omitempty"`
	Error  *Error                 `json:"error,omitempty"`
}

// RemoteRemoteRelationResults has a set of remote relation results.
type RegisterRemoteRelationResults struct {
	Results []RegisterRemoteRelationResult `json:"results,omitempty"`
}

// RemoteRelationDetails holds a remote relation token and corresponding macaroon.
type RemoteRelationDetails struct {
	Token    string             `json:"relation-token"`
	Macaroon *macaroon.Macaroon `json:"macaroon,omitempty"`
}

// RemoteEntityArgs holds arguments to an API call dealing with remote relations.
type RemoteEntityArgs struct {
	Args []RemoteEntityArg `json:"args"`
}

// RemoteEntityArg holds a remote relation token corresponding macaroons.
type RemoteEntityArg struct {
	Token     string         `json:"relation-token"`
	Macaroons macaroon.Slice `json:"macaroons,omitempty"`
}

// OfferArgs holds arguments to an API call dealing with offers.
type OfferArgs struct {
	Args []OfferArg `json:"args"`
}

// OfferArg holds an offer uuid and corresponding macaroons.
type OfferArg struct {
	OfferUUID string         `json:"offer-uuid"`
	Macaroons macaroon.Slice `json:"macaroons,omitempty"`
}

// RemoteApplicationInfo has attributes for a remote application.
type RemoteApplicationInfo struct {
	ModelTag    string `json:"model-tag"`
	Name        string `json:"name"`
	Description string `json:"description"`
	OfferURL    string `json:"offer-url"`
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

// ConsumeOfferDetails contains the details necessary to
// consume an application offer.
type ConsumeOfferDetails struct {
	Offer          *ApplicationOfferDetails `json:"offer,omitempty"`
	Macaroon       *macaroon.Macaroon       `json:"macaroon,omitempty"`
	ControllerInfo *ExternalControllerInfo  `json:"external-controller,omitempty"`
}

// ConsumeOfferDetailsResult contains the details necessary to
// consume an application offer or an error.
type ConsumeOfferDetailsResult struct {
	ConsumeOfferDetails
	Error *Error `json:"error,omitempty"`
}

// ConsumeOfferDetailsResults represents the result of a
// ConsumeOfferDetails call.
type ConsumeOfferDetailsResults struct {
	Results []ConsumeOfferDetailsResult `json:"results,omitempty"`
}

// RemoteEntities identifies multiple remote entities.
type RemoteEntities struct {
	Tokens []string `json:"tokens"`
}

// RelationUnit holds a remote relation token and a unit tag.
type RemoteRelationUnit struct {
	RelationToken string         `json:"relation-token"`
	Unit          string         `json:"unit"`
	Macaroons     macaroon.Slice `json:"macaroons,omitempty"`
}

// RemoteRelationUnits identifies multiple remote relation units.
type RemoteRelationUnits struct {
	RelationUnits []RemoteRelationUnit `json:"relation-units"`
}

// ModifyModelAccessRequest holds the parameters for making grant and revoke offer calls.
type ModifyOfferAccessRequest struct {
	Changes []ModifyOfferAccess `json:"changes"`
}

// ModifyOfferAccess contains parameters to grant and revoke access to an offer.
type ModifyOfferAccess struct {
	UserTag  string                `json:"user-tag"`
	Action   OfferAction           `json:"action"`
	Access   OfferAccessPermission `json:"access"`
	OfferURL string                `json:"offer-url"`
}

// OfferAction is an action that can be performed on an offer.
type OfferAction string

// Actions that can be preformed on an offer.
const (
	GrantOfferAccess  OfferAction = "grant"
	RevokeOfferAccess OfferAction = "revoke"
)

// OfferAccessPermission defines a type for an access permission on an offer.
type OfferAccessPermission string

// Access permissions that may be set on an offer.
const (
	OfferAdminAccess   OfferAccessPermission = "admin"
	OfferConsumeAccess OfferAccessPermission = "consume"
	OfferReadAccess    OfferAccessPermission = "read"
)

// ExternalControllerInfo holds addressed and other information
// needed to make a connection to an external controller.
type ExternalControllerInfo struct {
	ControllerTag string   `json:"controller-tag"`
	Alias         string   `json:"controller-alias"`
	Addrs         []string `json:"addrs"`
	CACert        string   `json:"ca-cert"`
}
