// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/proxy"
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
	ModelTag           string                `json:"model-tag"`
	Life               life.Value            `json:"life"`
	Type               string                `json:"type"`
	HostedMachineCount int                   `json:"hosted-machine-count"`
	ApplicationCount   int                   `json:"application-count"`
	UnitCount          int                   `json:"unit-count"`
	OwnerTag           string                `json:"owner-tag"`
	Machines           []ModelMachineInfo    `json:"machines,omitempty"`
	Volumes            []ModelVolumeInfo     `json:"volumes,omitempty"`
	Filesystems        []ModelFilesystemInfo `json:"filesystems,omitempty"`
	Error              *Error                `json:"error,omitempty"`
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

// ControllerVersionResults holds the results from an api call
// to get the controller's version information.
type ControllerVersionResults struct {
	Version   string `json:"version"`
	GitCommit string `json:"git-commit"`
}

const (
	// DashboardConnectionTypeProxy is the type key used for proxy connections
	DashboardConnectionTypeProxy = "proxy"

	// DashboardConnectionTypeSSHTunnel is the type key used for ssh connections
	DashboardConnectionTypeSSHTunnel = "ssh-tunnel"
)

// DashboardConnectionProxy represents a proxy connection to the Juju Dashboard
type DashboardConnectionProxy struct {
	Proxier       proxy.Proxier   `json:"proxier"`
	proxierConfig json.RawMessage `json:"-"`
	ProxierType   string          `json:"type"`
}

// ProxierFactory is an interface type representing a factory that can make a
// new juju proxier from the supplied type and JSON config.
type ProxierFactory interface {
	ProxierFromJSONDataBag(proxierType string, config json.RawMessage) (proxy.Proxier, error)
}

// DashboardConnectionSSHTunnel represents an ssh tunnel connection to the Juju
// Dashboard
type DashboardConnectionSSHTunnel struct {
	Host string `json:"host"`
	Port string `json:"port"`
}

// DashboardConnection interface represents a generic interface for establishing
// dashboard connections.
type DashboardConnection interface {
	Type() string
}

// DashboardConnectionInfo holds the information necassery
type DashboardConnectionInfo struct {
	Connection     DashboardConnection `json:"connection"`
	ConnectionType string              `json:"connection-type"`
	Error          *Error              `json:"error,omitempty"`
}

// DashboardInfo holds the results from an api call
// to get address info for the juju dashboard.
type DashboardInfo struct {
	Addresses []string `json:"addresses"`
	UseTunnel bool     `json:"use-tunnel"`
	Error     *Error   `json:"error,omitempty"`
}

// NewDashboardConnectionProxy constructs a new DashboardConnectionProxy from
// the supplied proxier.
func NewDashboardConnectionProxy(proxier proxy.Proxier) *DashboardConnectionProxy {
	proxierType := ""
	if proxier != nil {
		proxierType = proxier.Type()
	}
	return &DashboardConnectionProxy{
		Proxier:     proxier,
		ProxierType: proxierType,
	}
}

// Type implements the DashboardConnection interface
func (_ *DashboardConnectionProxy) Type() string {
	return DashboardConnectionTypeProxy
}

// Type implements the DashboardConnection interface
func (_ *DashboardConnectionSSHTunnel) Type() string {
	return DashboardConnectionTypeSSHTunnel
}

// UnmarshalJSON implements the encoding/json Unmarshaller interface
func (d *DashboardConnectionInfo) UnmarshalJSON(data []byte) error {
	wireFormat := &struct {
		Connection     json.RawMessage `json:"connection"`
		ConnectionType string          `json:"connection-type"`
		Error          *Error          `json:"error,omitempty"`
	}{}
	if err := json.Unmarshal(data, wireFormat); err != nil {
		return errors.Trace(err)
	}
	switch ct := wireFormat.ConnectionType; ct {
	case DashboardConnectionTypeProxy:
		con := &DashboardConnectionProxy{}
		if err := json.Unmarshal(wireFormat.Connection, con); err != nil {
			return errors.Trace(err)
		}
		d.Connection = con
	case DashboardConnectionTypeSSHTunnel:
		con := &DashboardConnectionSSHTunnel{}
		if err := json.Unmarshal(wireFormat.Connection, con); err != nil {
			return errors.Trace(err)
		}
		d.Connection = con
	case "":
		break
	default:
		return fmt.Errorf("Unknown connection type %q", ct)
	}

	d.ConnectionType = wireFormat.ConnectionType
	d.Error = wireFormat.Error
	return nil
}

// ProxierFromFactory attempts to construct a Juju proxier from the raw JSON
// configuration in this connection using the supplied proxier factory.
func (d *DashboardConnectionProxy) ProxierFromFactory(factory ProxierFactory) (proxy.Proxier, error) {
	if d.Proxier != nil {
		return d.Proxier, nil
	}

	if d.ProxierType == "" {
		return nil, errors.NotValidf("proxier type is empty, unable to construct a proxy from an empty type")
	}

	proxier, err := factory.ProxierFromJSONDataBag(d.ProxierType, d.proxierConfig)
	if err != nil {
		return nil, errors.Annotate(err, "making proxy from configuration")
	}

	d.Proxier = proxier
	return proxier, nil
}

// UnmarshalJSON implements the encoding/json Unmarshaller interface
func (d *DashboardConnectionProxy) UnmarshalJSON(data []byte) error {
	wireFormat := &struct {
		ProxierConfig json.RawMessage `json:"proxier"`
		ProxierType   string          `json:"type"`
	}{}

	if err := json.Unmarshal(data, wireFormat); err != nil {
		return errors.Trace(err)
	}

	d.ProxierType = wireFormat.ProxierType
	d.proxierConfig = wireFormat.ProxierConfig
	d.Proxier = nil

	return nil
}
