// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/permission"
)

// DestroyControllerArgs holds the arguments for destroying a controller.
type DestroyControllerArgs struct {
	// DestroyModels specifies whether or not the hosted models
	// should be destroyed as well. If this is not specified, and there are
	// other hosted models, the destruction of the controller will fail.
	DestroyModels bool `json:"destroy-models"`

	// DestroyStorage controls whether or not storage in the model (and
	// hosted models, if DestroyModels is true) should be destroyed.
	//
	// This is ternary: nil, false, or true. If nil and there is persistent
	// storage in the model (or hosted models), an error with the code
	// params.CodeHasPersistentStorage will be returned.
	DestroyStorage *bool `json:"destroy-storage,omitempty"`

	// Force specifies whether hosted model destruction will be forced,
	// i.e. keep going despite operational errors.
	Force *bool `json:"force,omitempty"`

	// MaxWait specifies the amount of time that each hosted model destroy step
	// will wait before forcing the next step to kick-off.
	// This parameter only makes sense in combination with 'force' set to 'true'.
	MaxWait *time.Duration `json:"max-wait,omitempty"`

	// ModelTimeout specifies how long to wait for each hosted model destroy process.
	ModelTimeout *time.Duration `json:"model-timeout,omitempty"`
}

// ModelBlockInfo holds information about a model and its
// current blocks.
type ModelBlockInfo struct {
	UUID      string   `json:"model-uuid"`
	Name      string   `json:"name"`
	Qualifier string   `json:"qualifier"`
	Blocks    []string `json:"blocks"`
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
	ModelTag           string                 `json:"model-tag"`
	Qualifier          string                 `json:"qualifier"`
	Life               life.Value             `json:"life"`
	Type               string                 `json:"type"`
	HostedMachineCount int                    `json:"hosted-machine-count"`
	ApplicationCount   int                    `json:"application-count"`
	UnitCount          int                    `json:"unit-count"`
	Applications       []ModelApplicationInfo `json:"applications,omitempty"`
	Machines           []ModelMachineInfo     `json:"machines,omitempty"`
	Volumes            []ModelVolumeInfo      `json:"volumes,omitempty"`
	Filesystems        []ModelFilesystemInfo  `json:"filesystems,omitempty"`
	Error              *Error                 `json:"error,omitempty"`
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

// ControllerConfigSet holds new config values for
// Controller.ConfigSet.
type ControllerConfigSet struct {
	Config map[string]interface{} `json:"config"`
}

// ControllerAction is an action that can be performed on a model.
type ControllerAction string

// Actions that can be preformed on a model.
const (
	GrantControllerAccess  ControllerAction = "grant"
	RevokeControllerAccess ControllerAction = "revoke"
)

func (c ControllerAction) AccessChange() permission.AccessChange {
	return permission.AccessChange(c)
}

// ControllerVersionResults holds the results from an api call
// to get the controller's version information.
type ControllerVersionResults struct {
	Version   string `json:"version"`
	GitCommit string `json:"git-commit"`
}

// DashboardConnectionSSHTunnel represents an ssh tunnel connection to the Juju
// Dashboard
type DashboardConnectionSSHTunnel struct {
	Model  string `json:"model,omitempty"`
	Entity string `json:"entity,omitempty"`
	Host   string `json:"host"`
	Port   string `json:"port"`
}

// DashboardConnectionInfo holds the information necessary
type DashboardConnectionInfo struct {
	ProxyConnection *Proxy                        `json:"proxy-connection"`
	SSHConnection   *DashboardConnectionSSHTunnel `json:"ssh-connection"`
	Error           *Error                        `json:"error,omitempty"`
}
