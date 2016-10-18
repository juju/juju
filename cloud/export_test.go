// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

import (
	"gopkg.in/yaml.v2"
)

var (
	FallbackPublicCloudInfo = fallbackPublicCloudInfo
)

func MarshalCredentials(credentialsMap map[string]CloudCredential) ([]byte, error) {
	var credentialsYAML struct {
		Credentials map[string]CloudCredential `yaml:"credentials"`
	}
	credentialsYAML.Credentials = credentialsMap
	return yaml.Marshal(credentialsYAML)
}
