// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version"
)

// ConfigValue encapsulates a configuration
// value and its source.
type ConfigValue struct {
	Value  interface{} `json:"value"`
	Source string      `json:"source"`
}

// ModelConfigResults contains the result of client API calls
// to get model config values.
type ModelConfigResults struct {
	Config map[string]ConfigValue `json:"config"`
}

// ModelDefaultsResult contains the result of client API calls to get the
// model default values.
type ModelDefaultsResult struct {
	Config map[string]ModelDefaults `json:"config"`
}

// ModelDefaults holds the settings for a given ModelDefaultsResult config
// attribute.
type ModelDefaults struct {
	Default    interface{}      `json:"default,omitempty"`
	Controller interface{}      `json:"controller,omitempty"`
	Regions    []RegionDefaults `json:"regions,omitempty"`
}

// RegionDefaults contains the settings for regions in a ModelDefaults.
type RegionDefaults struct {
	RegionName string      `json:"region-name"`
	Value      interface{} `json:"value"`
}

// ModelSet contains the arguments for ModelSet client API
// call.
type ModelSet struct {
	Config map[string]interface{} `json:"config"`
}

// ModelUnset contains the arguments for ModelUnset client API
// call.
type ModelUnset struct {
	Keys []string `json:"keys"`
}

// SetModelDefaults contains the arguments for SetModelDefaults
// client API call.
type SetModelDefaults struct {
	Config []ModelDefaultValues `json:"config"`
}

// ModelDefaultValues contains the default model values for
// a cloud/region.
type ModelDefaultValues struct {
	CloudTag    string                 `json:"cloud-tag,omitempty"`
	CloudRegion string                 `json:"cloud-region,omitempty"`
	Config      map[string]interface{} `json:"config"`
}

// ModelUnsetKeys contains the config keys to unset for
// a cloud/region.
type ModelUnsetKeys struct {
	CloudTag    string   `json:"cloud-tag,omitempty"`
	CloudRegion string   `json:"cloud-region,omitempty"`
	Keys        []string `json:"keys"`
}

// UnsetModelDefaults contains the arguments for UnsetModelDefaults
// client API call.
type UnsetModelDefaults struct {
	Keys []ModelUnsetKeys `json:"keys"`
}

// SetModelAgentVersion contains the arguments for
// SetModelAgentVersion client API call.
type SetModelAgentVersion struct {
	Version version.Number `json:"version"`
}

// ModelInfo holds information about the Juju model.
type ModelInfo struct {
	Name               string `json:"name"`
	UUID               string `json:"uuid"`
	ControllerUUID     string `json:"controller-uuid"`
	ProviderType       string `json:"provider-type"`
	DefaultSeries      string `json:"default-series"`
	Cloud              string `json:"cloud"`
	CloudRegion        string `json:"cloud-region,omitempty"`
	CloudCredentialTag string `json:"cloud-credential-tag,omitempty"`

	// OwnerTag is the tag of the user that owns the model.
	OwnerTag string `json:"owner-tag"`

	// Life is the current lifecycle state of the model.
	Life Life `json:"life"`

	// Status is the current status of the model.
	Status EntityStatus `json:"status"`

	// Users contains information about the users that have access
	// to the model. Owners and administrators can see all users
	// that have access; other users can only see their own details.
	Users []ModelUserInfo `json:"users"`
}

// ModelInfoResult holds the result of a ModelInfo call.
type ModelInfoResult struct {
	Result *ModelInfo `json:"result,omitempty"`
	Error  *Error     `json:"error,omitempty"`
}

// ModelInfoResult holds the result of a bulk ModelInfo call.
type ModelInfoResults struct {
	Results []ModelInfoResult `json:"results"`
}

// ModelInfoList holds a list of ModelInfo structures.
type ModelInfoList struct {
	Models []ModelInfo `json:"models,omitempty"`
}

// ModelInfoListResult holds the result of a call that returns a list
// of ModelInfo structures.
type ModelInfoListResult struct {
	Result *ModelInfoList `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// ModelInfoListResults holds the result of a bulk call that returns
// multiple lists of ModelInfo structures.
type ModelInfoListResults struct {
	Results []ModelInfoListResult `json:"results"`
}

// ModelUserInfo holds information on a user who has access to a
// model. Owners of a model can see this information for all users
// who have access, so it should not include sensitive information.
type ModelUserInfo struct {
	UserName       string               `json:"user"`
	DisplayName    string               `json:"display-name"`
	LastConnection *time.Time           `json:"last-connection"`
	Access         UserAccessPermission `json:"access"`
}

// ModelUserInfoResult holds the result of an ModelUserInfo call.
type ModelUserInfoResult struct {
	Result *ModelUserInfo `json:"result,omitempty"`
	Error  *Error         `json:"error,omitempty"`
}

// ModelUserInfoResults holds the result of a bulk ModelUserInfo API call.
type ModelUserInfoResults struct {
	Results []ModelUserInfoResult `json:"results"`
}

// ModifyModelAccessRequest holds the parameters for making grant and revoke model calls.
type ModifyModelAccessRequest struct {
	Changes []ModifyModelAccess `json:"changes"`
}

type ModifyModelAccess struct {
	UserTag  string               `json:"user-tag"`
	Action   ModelAction          `json:"action"`
	Access   UserAccessPermission `json:"access"`
	ModelTag string               `json:"model-tag"`
}

// ModelAction is an action that can be performed on a model.
type ModelAction string

// Actions that can be preformed on a model.
const (
	GrantModelAccess  ModelAction = "grant"
	RevokeModelAccess ModelAction = "revoke"
)

// UserAccessPermission is the type of permission that a user has to access a
// model.
type UserAccessPermission string

// Model access permissions that may be set on a user.
const (
	ModelAdminAccess UserAccessPermission = "admin"
	ModelReadAccess  UserAccessPermission = "read"
	ModelWriteAccess UserAccessPermission = "write"
)
