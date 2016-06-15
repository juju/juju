// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version"
)

// ModelConfigResults contains the result of client API calls
// to get model config values.
type ModelConfigResults struct {
	Config map[string]interface{} `json:"config"`
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

// SetModelAgentVersion contains the arguments for
// SetModelAgentVersion client API call.
type SetModelAgentVersion struct {
	Version version.Number `json:"version"`
}

// ModelInfo holds information about the Juju model.
type ModelInfo struct {
	// The json names for the fields below are as per the older
	// field names for backward compatability. New fields are
	// camel-cased for consistency within this type only.
	Name           string `json:"name"`
	UUID           string `json:"uuid"`
	ControllerUUID string `json:"controller-uuid"`
	ProviderType   string `json:"provider-type"`
	DefaultSeries  string `json:"default-series"`
	CloudRegion    string `json:"cloud-region,omitempty"`
	// TODO(axw) make sure we're setting this everywhere
	CloudCredential string `json:"cloud-credential,omitempty"`

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
	UserName       string                `json:"user"`
	DisplayName    string                `json:"display-name"`
	LastConnection *time.Time            `json:"last-connection"`
	Access         ModelAccessPermission `json:"access"`
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
	UserTag  string                `json:"user-tag"`
	Action   ModelAction           `json:"action"`
	Access   ModelAccessPermission `json:"access"`
	ModelTag string                `json:"model-tag"`
}

// ModelAction is an action that can be performed on a model.
type ModelAction string

// Actions that can be preformed on a model.
const (
	GrantModelAccess  ModelAction = "grant"
	RevokeModelAccess ModelAction = "revoke"
)

// ModelAccessPermission is the type of permission that a user has to access a
// model.
type ModelAccessPermission string

// Model access permissions that may be set on a user.
const (
	ModelReadAccess  ModelAccessPermission = "read"
	ModelWriteAccess ModelAccessPermission = "write"
)
