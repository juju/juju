// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

const (
	// OperatorInfoFile is the file which contains certificate information for
	// the operator.
	OperatorInfoFile = "operator.yaml"

	// OperatorClientInfoFile is the file containing info about the operator,
	// copied to the workload pod so the hook tools and juju-exec can function.
	OperatorClientInfoFile = "operator-client.yaml"

	// OperatorClientInfoCacheFile is a cache of OperatorClientInfoFile stored on the operator.
	OperatorClientInfoCacheFile = "operator-client-cache.yaml"

	// CACertFile is the file containing the cluster CA.
	CACertFile = "ca.crt"

	// InitContainerName is the name of the init container on workloads pods.
	InitContainerName = "juju-pod-init"
)

// OperatorInfo contains information needed by CAAS operators
type OperatorInfo struct {
	CACert     string `yaml:"ca-cert,omitempty"`
	Cert       string `yaml:"cert,omitempty"`
	PrivateKey string `yaml:"private-key,omitempty"`
}

// UnmarshalOperatorInfo parses OperatorInfo yaml data.
func UnmarshalOperatorInfo(data []byte) (*OperatorInfo, error) {
	var oi OperatorInfo
	err := yaml.Unmarshal(data, &oi)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &oi, nil
}

// Marshal OperatorInfo into yaml data.
func (info OperatorInfo) Marshal() ([]byte, error) {
	return yaml.Marshal(info)
}

// OperatorClientInfo contains information needed by CAAS tools.
type OperatorClientInfo struct {
	ServiceAddress string `yaml:"service-address,omitempty"`
	Token          string `yaml:"token,omitempty"`
}

// UnmarshalOperatorClientInfo parses OperatorClientInfo yaml data.
func UnmarshalOperatorClientInfo(data []byte) (*OperatorClientInfo, error) {
	var oi OperatorClientInfo
	err := yaml.Unmarshal(data, &oi)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &oi, nil
}

// Marshal OperatorClientInfo into yaml data.
func (info OperatorClientInfo) Marshal() ([]byte, error) {
	return yaml.Marshal(info)
}
