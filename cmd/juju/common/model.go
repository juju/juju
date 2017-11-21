// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package common

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/status"
)

// ModelInfo contains information about a model.
type ModelInfo struct {
	// Name is a fully qualified model name, i.e. having the format $owner/$model.
	Name string `json:"name" yaml:"name"`

	// ShortName is un-qualified model name.
	ShortName      string                      `json:"short-name" yaml:"short-name"`
	UUID           string                      `json:"model-uuid" yaml:"model-uuid"`
	ControllerUUID string                      `json:"controller-uuid" yaml:"controller-uuid"`
	ControllerName string                      `json:"controller-name" yaml:"controller-name"`
	Owner          string                      `json:"owner" yaml:"owner"`
	Cloud          string                      `json:"cloud" yaml:"cloud"`
	CloudRegion    string                      `json:"region,omitempty" yaml:"region,omitempty"`
	ProviderType   string                      `json:"type,omitempty" yaml:"type,omitempty"`
	Life           string                      `json:"life" yaml:"life"`
	Status         *ModelStatus                `json:"status,omitempty" yaml:"status,omitempty"`
	Users          map[string]ModelUserInfo    `json:"users,omitempty" yaml:"users,omitempty"`
	Machines       map[string]ModelMachineInfo `json:"machines,omitempty" yaml:"machines,omitempty"`
	SLA            string                      `json:"sla,omitempty" yaml:"sla,omitempty"`
	SLAOwner       string                      `json:"sla-owner,omitempty" yaml:"sla-owner,omitempty"`
	AgentVersion   string                      `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
}

// ModelMachineInfo contains information about a machine in a model.
// We currently only care about showing core count, but might
// in the future care about memory, disks, containers etc.
type ModelMachineInfo struct {
	Cores uint64 `json:"cores" yaml:"cores"`
}

// ModelStatus contains the current status of a model.
type ModelStatus struct {
	Current        status.Status `json:"current,omitempty" yaml:"current,omitempty"`
	Message        string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since          string        `json:"since,omitempty" yaml:"since,omitempty"`
	Migration      string        `json:"migration,omitempty" yaml:"migration,omitempty"`
	MigrationStart string        `json:"migration-start,omitempty" yaml:"migration-start,omitempty"`
	MigrationEnd   string        `json:"migration-end,omitempty" yaml:"migration-end,omitempty"`
}

// ModelUserInfo defines the serialization behaviour of the model user
// information.
type ModelUserInfo struct {
	DisplayName    string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access         string `yaml:"access" json:"access"`
	LastConnection string `yaml:"last-connection" json:"last-connection"`
}

// friendlyDuration renders a time pointer that we get from the API as
// a friendly string.
func friendlyDuration(when *time.Time, now time.Time) string {
	if when == nil {
		return ""
	}
	return UserFriendlyDuration(*when, now)
}

// ModelInfoFromParams translates a params.ModelInfo to ModelInfo.
func ModelInfoFromParams(info params.ModelInfo, now time.Time) (ModelInfo, error) {
	ownerTag, err := names.ParseUserTag(info.OwnerTag)
	if err != nil {
		return ModelInfo{}, errors.Trace(err)
	}
	cloudTag, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		return ModelInfo{}, errors.Trace(err)
	}
	modelInfo := ModelInfo{
		ShortName:      info.Name,
		Name:           jujuclient.JoinOwnerModelName(ownerTag, info.Name),
		UUID:           info.UUID,
		ControllerUUID: info.ControllerUUID,
		Owner:          ownerTag.Id(),
		Life:           string(info.Life),
		Cloud:          cloudTag.Id(),
		CloudRegion:    info.CloudRegion,
	}
	if info.AgentVersion != nil {
		modelInfo.AgentVersion = info.AgentVersion.String()
	}
	// Although this may be more performance intensive, we have to use reflection
	// since structs containing map[string]interface {} cannot be compared, i.e
	// cannot use simple '==' here.
	if !reflect.DeepEqual(info.Status, params.EntityStatus{}) {
		modelInfo.Status = &ModelStatus{
			Current: info.Status.Status,
			Message: info.Status.Info,
			Since:   friendlyDuration(info.Status.Since, now),
		}
	}
	if info.Migration != nil {
		status := modelInfo.Status
		if status == nil {
			status = &ModelStatus{}
			modelInfo.Status = status
		}
		status.Migration = info.Migration.Status
		status.MigrationStart = friendlyDuration(info.Migration.Start, now)
		status.MigrationEnd = friendlyDuration(info.Migration.End, now)
	}

	if info.ProviderType != "" {
		modelInfo.ProviderType = info.ProviderType

	}
	if len(info.Users) != 0 {
		modelInfo.Users = ModelUserInfoFromParams(info.Users, now)
	}
	if len(info.Machines) != 0 {
		modelInfo.Machines = ModelMachineInfoFromParams(info.Machines)
	}
	if info.SLA != nil {
		modelInfo.SLA = modelSLAFromParams(info.SLA)
		modelInfo.SLAOwner = modelSLAOwnerFromParams(info.SLA)
	}
	return modelInfo, nil
}

// ModelMachineInfoFromParams translates []params.ModelMachineInfo to a map of
// machine ids to ModelMachineInfo.
func ModelMachineInfoFromParams(machines []params.ModelMachineInfo) map[string]ModelMachineInfo {
	output := make(map[string]ModelMachineInfo, len(machines))
	for _, info := range machines {
		mInfo := ModelMachineInfo{}
		if info.Hardware != nil && info.Hardware.Cores != nil {
			mInfo.Cores = *info.Hardware.Cores
		}
		output[info.Id] = mInfo
	}
	return output
}

// ModelUserInfoFromParams translates []params.ModelUserInfo to a map of
// user names to ModelUserInfo.
func ModelUserInfoFromParams(users []params.ModelUserInfo, now time.Time) map[string]ModelUserInfo {
	output := make(map[string]ModelUserInfo)
	for _, info := range users {
		outInfo := ModelUserInfo{
			DisplayName: info.DisplayName,
			Access:      string(info.Access),
		}
		if info.LastConnection != nil {
			outInfo.LastConnection = UserFriendlyDuration(*info.LastConnection, now)
		} else {
			outInfo.LastConnection = "never connected"
		}
		output[names.NewUserTag(info.UserName).Id()] = outInfo
	}
	return output
}

func modelSLAFromParams(sla *params.ModelSLAInfo) string {
	if sla == nil {
		return ""
	}
	return sla.Level
}

func modelSLAOwnerFromParams(sla *params.ModelSLAInfo) string {
	if sla == nil {
		return ""
	}
	return sla.Owner
}

// OwnerQualifiedModelName returns the model name qualified with the
// model owner.
func OwnerQualifiedModelName(modelName string, owner, user names.UserTag) string {
	if jujuclient.IsQualifiedModelName(modelName) {
		return modelName
	}
	return jujuclient.JoinOwnerModelName(owner, modelName)
}
