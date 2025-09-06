// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package specs

import (
	core "k8s.io/api/core/v1"
)

// K8sSecret is a subset of v1.Secret which defines
// attributes we expose for charms to set.
type K8sSecret struct {
	Name        string            `json:"name" yaml:"name"`
	Type        core.SecretType   `json:"type" yaml:"type"`
	Annotations map[string]string `json:"annotations,omitempty"`
	Data        map[string]string `json:"data,omitempty" yaml:"data,omitempty"`
	StringData  map[string]string `json:"stringData,omitempty" yaml:"stringData,omitempty"`
}
