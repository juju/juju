// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// DestroyControllerArgs holds the arguments for destroying a controller.
type DestroyControllerArgs struct {
	// DestroyModels specifies whether or not the hosted models
	// should be destroyed as well. If this is not specified, and there are
	// other hosted models, the destruction of the controller will fail.
	DestroyModels bool `json:"destroy-models"`
}

// ModelBlockInfo holds information about an model and its
// current blocks.
type ModelBlockInfo struct {
	Name     string   `json:"name"`
	UUID     string   `json:"model-uuid"`
	OwnerTag string   `json:"owner-tag"`
	Blocks   []string `json:"blocks"`
}

// ModelBlockInfoList holds information about the blocked models
// for a controller.
type ModelBlockInfoList struct {
	Models []ModelBlockInfo `json:"models,omitempty"`
}

// RemoveBlocksArgs holds the arguments for the RemoveBlocks command. It is a
// struct to facilitate the easy addition of being able to remove blocks for
// individual models at a later date.
type RemoveBlocksArgs struct {
	All bool `json:"all"`
}

// ModelStatus holds information about the status of a juju model.
type ModelStatus struct {
	ModelTag           string `json:"model-tag"`
	Life               Life   `json:"life"`
	HostedMachineCount int    `json:"hosted-machine-count"`
	ApplicationCount   int    `json:"application-count"`
	OwnerTag           string `json:"owner-tag"`
}

// ModelStatusResults holds status information about a group of models.
type ModelStatusResults struct {
	Results []ModelStatus `json:"models"`
}

// ModifyControllerAccessRequest holds the parameters for making grant and revoke controller calls.
type ModifyControllerAccessRequest struct {
	Changes []ModifyControllerAccess `json:"changes"`
}

type ModifyControllerAccess struct {
	UserTag string           `json:"user-tag"`
	Action  ControllerAction `json:"action"`
	Access  string           `json:"access"`
}

// UserAccess holds the level of access a user
// has on a controller or model.
type UserAccess struct {
	UserTag string `json:"user-tag"`
	Access  string `json:"access"`
}

// UserAccessResult holds an access level for
// a user, or an error.
type UserAccessResult struct {
	Result *UserAccess `json:"result,omitempty"`
	Error  *Error      `json:"error,omitempty"`
}

// UserAccessResults holds the results of an api
// call to look up access for users.
type UserAccessResults struct {
	Results []UserAccessResult `json:"results,omitempty"`
}

// ControllerAction is an action that can be performed on a model.
type ControllerAction string

// Actions that can be preformed on a model.
const (
	GrantControllerAccess  ControllerAction = "grant"
	RevokeControllerAccess ControllerAction = "revoke"
)
