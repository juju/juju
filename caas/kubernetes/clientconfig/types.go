// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package clientconfig

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
)

// ClientConfig - a set of cloud endpoint info and user credentials
// Clouds and user Credentials are joined by
// Contexts. There should always be a valid Context with same name as
// the CurrentContext string.
type ClientConfig struct {
	Type           string
	Contexts       map[string]Context
	CurrentContext string
	Clouds         map[string]CloudConfig
	Credentials    map[string]cloud.Credential
}

// Context joins Clouds and Credentials.
type Context struct {
	CloudName      string
	CredentialName string
}

// CloudConfig stores information about how to connect to a Cloud.
type CloudConfig struct {
	Endpoint   string
	Attributes map[string]interface{}
}

// If existing CAAS cloud has Cluster_A and User_A, here's what happens when we try to define a new CAAS cloud:

// Cluster_B, User_B: New Cloud & new Credential for that cloud
// Cluster B, User A: New Cloud & New Credential for that cloud (duplicate is necessary)
// Cluster_A, User_A: error. already exists.
// Cluster_A, User_B: No new Cloud, new Credential for the cloud.

// ClientConfigFunc is a function that returns a ClientConfig. Functions of this type should be available for each supported CAAS framework, e.g. Kubernetes.
type ClientConfigFunc func(io.Reader) (*ClientConfig, error)

// NewClientConfigReader returns a function of type ClientConfigFunc to read the client config for a given cloud type.
func NewClientConfigReader(cloudType string) (ClientConfigFunc, error) {
	switch cloudType {
	case "kubernetes":
		return NewK8sClientConfig, nil
	default:
		return nil, errors.Errorf("Cannot read local config: unsupported cloud type '%s'", cloudType)
	}
}
