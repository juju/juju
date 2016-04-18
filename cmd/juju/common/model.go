// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package common

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/status"
	"github.com/juju/names"
)

// ModelInfo contains information about a model.
type ModelInfo struct {
	Name           string                   `json:"name" yaml:"name"`
	UUID           string                   `json:"model-uuid" yaml:"model-uuid"`
	ControllerUUID string                   `json:"controller-uuid" yaml:"controller-uuid"`
	Owner          string                   `json:"owner" yaml:"owner"`
	ProviderType   string                   `json:"type" yaml:"type"`
	Life           string                   `json:"life" yaml:"life"`
	Status         ModelStatus              `json:"status" yaml:"status"`
	Users          map[string]ModelUserInfo `json:"users" yaml:"users"`
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
	return ModelInfo{
		Name:           info.Name,
		UUID:           info.UUID,
		ControllerUUID: info.ControllerUUID,
		Owner:          tag.Id(),
		Life:           string(info.Life),
		Status:         status,
		ProviderType:   info.ProviderType,
		Users:          ModelUserInfoFromParams(info.Users, now),
	}, nil
}

// ModelUserInfoFromParams translates []params.ModelInfo to a map of
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
