// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package common

import (
	"fmt"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/status"
	"gopkg.in/juju/names.v2"
)

// ModelInfo contains information about a model.
type ModelInfo struct {
	Name           string                      `json:"name" yaml:"name"`
	UUID           string                      `json:"model-uuid" yaml:"model-uuid"`
	ControllerUUID string                      `json:"controller-uuid" yaml:"controller-uuid"`
	ControllerName string                      `json:"controller-name" yaml:"controller-name"`
	Owner          string                      `json:"owner" yaml:"owner"`
	Cloud          string                      `json:"cloud" yaml:"cloud"`
	CloudRegion    string                      `json:"region,omitempty" yaml:"region,omitempty"`
	ProviderType   string                      `json:"type" yaml:"type"`
	Life           string                      `json:"life" yaml:"life"`
	Status         ModelStatus                 `json:"status" yaml:"status"`
	Users          map[string]ModelUserInfo    `json:"users" yaml:"users"`
	Machines       map[string]ModelMachineInfo `json:"machines,omitempty" yaml:"machines,omitempty"`
}

// ModelMachineInfo contains information about a machine in a model.
// We currently only care about showing core count, but might
// in the future care about memory, disks, containers etc.
type ModelMachineInfo struct {
	Cores uint64 `json:"cores" yaml:"cores"`
}

// ModelStatus contains the current status of a model.
type ModelStatus struct {
	Current status.Status `json:"current" yaml:"current"`
	Message string        `json:"message,omitempty" yaml:"message,omitempty"`
	Since   string        `json:"since,omitempty" yaml:"since,omitempty"`
}

// ModelUserInfo defines the serialization behaviour of the model user
// information.
type ModelUserInfo struct {
	DisplayName    string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access         string `yaml:"access" json:"access"`
	LastConnection string `yaml:"last-connection" json:"last-connection"`
}

// ModelInfoFromParams translates a params.ModelInfo to ModelInfo.
func ModelInfoFromParams(info params.ModelInfo, now time.Time) (ModelInfo, error) {
	tag, err := names.ParseUserTag(info.OwnerTag)
	if err != nil {
		return ModelInfo{}, errors.Trace(err)
	}
	status := ModelStatus{
		Current: info.Status.Status,
		Message: info.Status.Info,
	}
	if info.Status.Since != nil {
		status.Since = UserFriendlyDuration(*info.Status.Since, now)
	}
	cloudTag, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		return ModelInfo{}, errors.Trace(err)
	}
	return ModelInfo{
		Name:           info.Name,
		UUID:           info.UUID,
		ControllerUUID: info.ControllerUUID,
		Owner:          tag.Id(),
		Life:           string(info.Life),
		Status:         status,
		Cloud:          cloudTag.Id(),
		CloudRegion:    info.CloudRegion,
		ProviderType:   info.ProviderType,
		Users:          ModelUserInfoFromParams(info.Users, now),
		Machines:       ModelMachineInfoFromParams(info.Machines),
	}, nil
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
		output[names.NewUserTag(info.UserName).Canonical()] = outInfo
	}
	return output
}

// OwnerQualifiedModelName returns the model name qualified with the
// model owner if the owner is not the same as the given canonical
// user name. If the owner is a local user, we omit the domain.
func OwnerQualifiedModelName(modelName string, owner, user names.UserTag) string {
	if owner.Canonical() == user.Canonical() {
		return modelName
	}
	var ownerName string
	if owner.IsLocal() {
		ownerName = owner.Name()
	} else {
		ownerName = owner.Canonical()
	}
	return fmt.Sprintf("%s/%s", ownerName, modelName)
}
