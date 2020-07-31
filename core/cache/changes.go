// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cache

import (
	"time"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/settings"
	"github.com/juju/juju/core/status"
)

// ControllerConfigChange represents the initial controller config,
// or a change initiated by an admin updating the controller config.
type ControllerConfigChange struct {
	Config map[string]interface{}
}

// ModelChange represents either a new model, or a change
// to an existing model.
type ModelChange struct {
	ModelUUID       string
	Name            string
	Type            model.ModelType
	Life            life.Value
	Owner           string // tag maybe?
	IsController    bool
	Cloud           string
	CloudRegion     string
	CloudCredential string
	Annotations     map[string]string
	Config          map[string]interface{}
	Status          status.StatusInfo

	UserPermissions map[string]permission.Access
}

// RemoveModel represents the situation when a model is removed
// from the database.
type RemoveModel struct {
	ModelUUID string
}

// PodSpec is used to determine whether or not a CAAS application
// has called the command to set the pod spec.
type PodSpec struct {
	Spec    string
	Raw     bool
	Counter int
}

// ApplicationChange represents either a new application, or a change
// to an existing application in a model.
type ApplicationChange struct {
	ModelUUID       string
	Name            string
	Exposed         bool
	CharmURL        string
	Life            life.Value
	MinUnits        int
	Constraints     constraints.Value
	Annotations     map[string]string
	Config          map[string]interface{}
	Subordinate     bool
	Status          status.StatusInfo
	OperatorStatus  status.StatusInfo // For CAAS applications.
	WorkloadVersion string
	PodSpec         *PodSpec // For CAAS applications.
}

// copy returns a deep copy of the ApplicationChange.
func (a ApplicationChange) copy() ApplicationChange {
	cons := a.Constraints.String()
	a.Constraints = constraints.MustParse(cons)

	a.Annotations = copyStringMap(a.Annotations)
	a.Config = copyDataMap(a.Config)
	a.Status = copyStatusInfo(a.Status)

	return a
}

// RemoveApplication represents the situation when an application
// is removed from a model in the database.
type RemoveApplication struct {
	ModelUUID string
	Name      string
}

// CharmChange represents either a new charm, or a change
// to an existing charm in a model.
type CharmChange struct {
	ModelUUID     string
	CharmURL      string
	CharmVersion  string
	LXDProfile    lxdprofile.Profile
	DefaultConfig map[string]interface{}
}

func (c CharmChange) copy() CharmChange {
	var cpConfig map[string]string
	pConfig := c.LXDProfile.Config
	if pConfig != nil {
		cpConfig = make(map[string]string, len(pConfig))
		for k, v := range pConfig {
			cpConfig[k] = v
		}
	}
	c.LXDProfile.Config = cpConfig

	var cpDevices map[string]map[string]string
	pDevices := c.LXDProfile.Devices
	if pDevices != nil {
		cpDevices = make(map[string]map[string]string, len(pDevices))
		for dName, dCfg := range pDevices {
			var cCfg map[string]string
			if dCfg != nil {
				cCfg = make(map[string]string, len(dCfg))
				for k, v := range dCfg {
					cCfg[k] = v
				}
			}
			cpDevices[dName] = cCfg
		}
	}
	c.LXDProfile.Devices = cpDevices

	c.DefaultConfig = copyDataMap(c.DefaultConfig)

	return c
}

// RemoveCharm represents the situation when an charm
// is removed from a model in the database.
type RemoveCharm struct {
	ModelUUID string
	CharmURL  string
}

// UnitChange represents either a new unit, or a change
// to an existing unit in a model.
type UnitChange struct {
	ModelUUID                string
	Name                     string
	Application              string
	Series                   string
	Annotations              map[string]string
	CharmURL                 string
	Life                     life.Value
	PublicAddress            string
	PrivateAddress           string
	MachineId                string
	OpenPortRangesByEndpoint map[string][]network.PortRange
	Principal                string
	Subordinate              bool

	WorkloadStatus  status.StatusInfo
	AgentStatus     status.StatusInfo
	ContainerStatus status.StatusInfo // For CAAS models.
}

// copy returns a deep copy of the UnitChange.
func (u UnitChange) copy() UnitChange {
	u.OpenPortRangesByEndpoint = copyPortRangeMap(u.OpenPortRangesByEndpoint)
	u.Annotations = copyStringMap(u.Annotations)
	u.WorkloadStatus = copyStatusInfo(u.WorkloadStatus)
	u.AgentStatus = copyStatusInfo(u.AgentStatus)

	return u
}

// RemoveUnit represents the situation when a unit
// is removed from a model in the database.
type RemoveUnit struct {
	ModelUUID string
	Name      string
}

// RelationChange represents either a new relation, or a change
// to an existing relation in a model.
type RelationChange struct {
	ModelUUID string
	Key       string
	Endpoints []Endpoint
}

// Endpoint holds all relevant information about a relation endpoint.
type Endpoint struct {
	Application string
	Name        string
	Role        string
	Interface   string
	Optional    bool
	Limit       int
	Scope       string
}

// copy returns a deep copy of the RelationChange.
func (c RelationChange) copy() RelationChange {
	if existing := c.Endpoints; existing != nil {
		endpoints := make([]Endpoint, len(existing))
		for i, ep := range existing {
			endpoints[i] = ep
		}
		c.Endpoints = existing
	}
	return c
}

// RemoveRelation represents the situation when a relation
// is removed from a model in the database.
type RemoveRelation struct {
	ModelUUID string
	Key       string
}

// MachineChange represents either a new machine, or a change
// to an existing machine in a model.
type MachineChange struct {
	ModelUUID                string
	Id                       string
	InstanceId               string
	AgentStatus              status.StatusInfo
	InstanceStatus           status.StatusInfo
	Life                     life.Value
	Annotations              map[string]string
	Config                   map[string]interface{}
	Series                   string
	ContainerType            string
	IsManual                 bool
	SupportedContainers      []instance.ContainerType
	SupportedContainersKnown bool
	HardwareCharacteristics  *instance.HardwareCharacteristics
	CharmProfiles            []string
	Addresses                []network.ProviderAddress
	HasVote                  bool
	WantsVote                bool
}

// copy returns a deep copy of the MachineChange.
func (m MachineChange) copy() MachineChange {
	m.AgentStatus = copyStatusInfo(m.AgentStatus)
	m.InstanceStatus = copyStatusInfo(m.InstanceStatus)
	m.Annotations = copyStringMap(m.Annotations)
	m.Config = copyDataMap(m.Config)

	var cSupportedContainers []instance.ContainerType
	if m.SupportedContainers != nil {
		cSupportedContainers = make([]instance.ContainerType, len(m.SupportedContainers))
		for i, v := range m.SupportedContainers {
			cSupportedContainers[i] = v
		}
	}
	m.SupportedContainers = cSupportedContainers

	var cHardwareCharacteristics instance.HardwareCharacteristics
	if m.HardwareCharacteristics != nil {
		cHardwareCharacteristics = *m.HardwareCharacteristics
	}
	m.HardwareCharacteristics = &cHardwareCharacteristics

	var cCharmProfiles []string
	if m.CharmProfiles != nil {
		cCharmProfiles = make([]string, len(m.CharmProfiles))
		for i, v := range m.CharmProfiles {
			cCharmProfiles[i] = v
		}
	}
	m.CharmProfiles = cCharmProfiles

	var cAddresses []network.ProviderAddress
	if m.Addresses != nil {
		cAddresses = make([]network.ProviderAddress, len(m.Addresses))
		for i, v := range m.Addresses {
			cAddresses[i] = v
		}
	}
	m.Addresses = cAddresses

	return m
}

// RemoveMachine represents the situation when a machine
// is removed from a model in the database.
type RemoveMachine struct {
	ModelUUID string
	Id        string
}

// BranchChange represents a change to an active model branch.
// Note that this corresponds to a multi-watcher BranchInfo payload,
// and that the cache behaviour differs from other entities;
// when a generation is completed (aborted or committed),
// it is no longer an active branch and will be removed from the cache.
type BranchChange struct {
	ModelUUID     string
	Id            string
	Name          string
	AssignedUnits map[string][]string
	Config        map[string]settings.ItemChanges
	Created       int64
	CreatedBy     string
	Completed     int64
	CompletedBy   string
	GenerationId  int
}

func (b BranchChange) copy() BranchChange {
	var cAssignedUnits map[string][]string
	bAssignedUnits := b.AssignedUnits
	if bAssignedUnits != nil {
		cAssignedUnits = make(map[string][]string, len(bAssignedUnits))
		for k, v := range bAssignedUnits {
			units := make([]string, len(v))
			for i, u := range v {
				units[i] = u
			}
			cAssignedUnits[k] = units
		}
	}
	b.AssignedUnits = cAssignedUnits

	var cConfig map[string]settings.ItemChanges
	bConfig := b.Config
	if bConfig != nil {
		cConfig = make(map[string]settings.ItemChanges, len(bConfig))
		for k, v := range bConfig {
			changes := make(settings.ItemChanges, len(v))
			for i, ch := range v {
				changes[i] = settings.ItemChange{
					Type:     ch.Type,
					Key:      ch.Key,
					NewValue: ch.NewValue,
					OldValue: ch.OldValue,
				}
			}
			cConfig[k] = changes
		}
	}
	b.Config = cConfig

	return b
}

// RemoveBranch represents the situation when a branch is to be removed
// from the cache. This will rarely be a result of deletion from the database.
// It will usually be the result of the branch no longer being considered
// "in-flight" due to being committed or aborted.
type RemoveBranch struct {
	ModelUUID string
	Id        string
}

func copyStatusInfo(info status.StatusInfo) status.StatusInfo {
	var cSince *time.Time
	if info.Since != nil {
		s := *info.Since
		cSince = &s
	}

	return status.StatusInfo{
		Status:  info.Status,
		Message: info.Message,
		Data:    copyDataMap(info.Data),
		Since:   cSince,
	}
}

func copyDataMap(data map[string]interface{}) map[string]interface{} {
	var cData map[string]interface{}
	if data != nil {
		cData = make(map[string]interface{}, len(data))
		for i, d := range data {
			cData[i] = d
		}
	}
	return cData
}

func copyStringMap(data map[string]string) map[string]string {
	var cData map[string]string
	if data != nil {
		cData = make(map[string]string, len(data))
		for i, d := range data {
			cData[i] = d
		}
	}
	return cData
}

func copyPortRangeMap(data map[string][]network.PortRange) map[string][]network.PortRange {
	if data == nil {
		return nil
	}

	cData := make(map[string][]network.PortRange, len(data))
	for i, d := range data {
		cData[i] = append([]network.PortRange(nil), d...)
	}
	return cData
}
