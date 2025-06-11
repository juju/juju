// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for infos.

package common

import (
	"reflect"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v6"

	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/rpc/params"
)

// ModelInfo contains information about a model.
type ModelInfo struct {
	// Name is a fully qualified model name, i.e. having the format $qualifier/$model.
	Name string `json:"name" yaml:"name"`

	// ShortName is un-qualified model name.
	ShortName      string                       `json:"short-name" yaml:"short-name"`
	UUID           string                       `json:"model-uuid" yaml:"model-uuid"`
	Type           model.ModelType              `json:"model-type" yaml:"model-type"`
	ControllerUUID string                       `json:"controller-uuid" yaml:"controller-uuid"`
	ControllerName string                       `json:"controller-name" yaml:"controller-name"`
	IsController   bool                         `json:"is-controller" yaml:"is-controller"`
	Cloud          string                       `json:"cloud" yaml:"cloud"`
	CloudRegion    string                       `json:"region,omitempty" yaml:"region,omitempty"`
	ProviderType   string                       `json:"type,omitempty" yaml:"type,omitempty"`
	Life           string                       `json:"life" yaml:"life"`
	Status         *ModelStatus                 `json:"status,omitempty" yaml:"status,omitempty"`
	Users          map[string]ModelUserInfo     `json:"users,omitempty" yaml:"users,omitempty"`
	Machines       map[string]ModelMachineInfo  `json:"machines,omitempty" yaml:"machines,omitempty"`
	SecretBackends map[string]SecretBackendInfo `json:"secret-backends,omitempty" yaml:"secret-backends,omitempty"`
	AgentVersion   string                       `json:"agent-version,omitempty" yaml:"agent-version,omitempty"`
	Credential     *ModelCredential             `json:"credential,omitempty" yaml:"credential,omitempty"`

	SupportedFeatures []SupportedFeature `json:"supported-features,omitempty" yaml:"supported-features,omitempty"`
}

// SupportedFeature describes a feature that is supported by a particular model.
type SupportedFeature struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`

	// Version is optional; some features might simply be booleans with
	// no particular version attached.
	Version string `json:"version,omitempty" yaml:"version,omitempty"`
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
	Reason         string        `json:"reason,omitempty" yaml:"reason,omitempty"`
	Since          string        `json:"since,omitempty" yaml:"since,omitempty"`
	Migration      string        `json:"migration,omitempty" yaml:"migration,omitempty"`
	MigrationStart string        `json:"migration-start,omitempty" yaml:"migration-start,omitempty"`
	MigrationEnd   string        `json:"migration-end,omitempty" yaml:"migration-end,omitempty"`
}

// SecretBackendInfo contains the current status of a secret backend.
type SecretBackendInfo struct {
	NumSecrets int    `yaml:"num-secrets" json:"num-secrets"`
	Status     string `yaml:"status" json:"status"`
	Message    string `yaml:"message,omitempty" json:"message,omitempty"`
}

// ModelUserInfo defines the serialization behaviour of the model user
// information.
type ModelUserInfo struct {
	DisplayName    string `yaml:"display-name,omitempty" json:"display-name,omitempty"`
	Access         string `yaml:"access" json:"access"`
	LastConnection string `yaml:"last-connection" json:"last-connection"`
}

// FriendlyDuration renders a time pointer that we get from the API as
// a friendly string.
func FriendlyDuration(when *time.Time, now time.Time) string {
	if when == nil {
		return ""
	}
	return UserFriendlyDuration(*when, now)
}

// ModelCredential contains model credential basic details.
type ModelCredential struct {
	Name     string `json:"name" yaml:"name"`
	Owner    string `json:"owner" yaml:"owner"`
	Cloud    string `json:"cloud" yaml:"cloud"`
	Validity string `json:"validity-check,omitempty" yaml:"validity-check,omitempty"`
}

// ModelStatusReason extracts the reason, if any, from a status data bag.
func ModelStatusReason(data map[string]interface{}) string {
	reason, _ := data["reason"].(string)
	return strings.Trim(reason, "\n")
}

// ModelInfoFromParams translates a params.ModelInfo to ModelInfo.
func ModelInfoFromParams(info params.ModelInfo, now time.Time) (ModelInfo, error) {
	cloudTag, err := names.ParseCloudTag(info.CloudTag)
	if err != nil {
		return ModelInfo{}, errors.Trace(err)
	}
	modelInfo := ModelInfo{
		ShortName:      info.Name,
		Name:           jujuclient.QualifyModelName(info.Qualifier, info.Name),
		Type:           model.ModelType(info.Type),
		UUID:           info.UUID,
		ControllerUUID: info.ControllerUUID,
		IsController:   info.IsController,
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
			Reason:  ModelStatusReason(info.Status.Data),
			Since:   FriendlyDuration(info.Status.Since, now),
		}
	}
	if info.Migration != nil {
		status := modelInfo.Status
		if status == nil {
			status = &ModelStatus{}
			modelInfo.Status = status
		}
		status.Migration = info.Migration.Status
		status.MigrationStart = FriendlyDuration(info.Migration.Start, now)
		status.MigrationEnd = FriendlyDuration(info.Migration.End, now)
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
	if len(info.SecretBackends) != 0 {
		modelInfo.SecretBackends = ModelSecretBackendInfoFromParams(info.SecretBackends)
	}

	if info.CloudCredentialTag != "" {
		credTag, err := names.ParseCloudCredentialTag(info.CloudCredentialTag)
		if err != nil {
			return ModelInfo{}, errors.Trace(err)
		}
		modelInfo.Credential = &ModelCredential{
			Name:     credTag.Name(),
			Owner:    credTag.Owner().Id(),
			Cloud:    credTag.Cloud().Id(),
			Validity: HumanReadableBoolPointer(info.CloudCredentialValidity, "valid", "invalid"),
		}
	}

	for _, feat := range info.SupportedFeatures {
		modelInfo.SupportedFeatures = append(modelInfo.SupportedFeatures,
			SupportedFeature{
				Name:        feat.Name,
				Description: feat.Description,
				Version:     feat.Version,
			},
		)
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

// ModelSecretBackendInfoFromParams translates []params.SecretBackendResult to a map of
// secret backend names to SecretBackendInfo.
func ModelSecretBackendInfoFromParams(backends []params.SecretBackendResult) map[string]SecretBackendInfo {
	output := make(map[string]SecretBackendInfo, len(backends))
	for _, info := range backends {
		bInfo := SecretBackendInfo{
			NumSecrets: info.NumSecrets,
			Status:     info.Status,
			Message:    info.Message,
		}
		output[info.Result.Name] = bInfo
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

// OwnerQualifiedModelName returns the model name qualified with the
// model qualifier if the qualifier is not the same as the given canonical
// user name.
func OwnerQualifiedModelName(modelName, qualifier string, user names.UserTag) string {
	if qualifier == user.Id() {
		return modelName
	}
	return jujuclient.QualifyModelName(qualifier, modelName)
}
