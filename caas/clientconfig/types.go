package caas

import (
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
)

type ClientConfig struct {
	Type           string
	Contexts       map[string]Context
	CurrentContext string
	Clouds         map[string]CloudConfig
	Credentials    map[string]cloud.Credential
}

type Context struct {
	CloudName      string
	CredentialName string
}

type CloudConfig struct {
	Endpoint   string
	Attributes map[string]interface{}
}

// If existing CAAS cloud has Cluster_A and User_A, here's what happens when we try to define a new CAAS cloud:

// Cluster_B, User_B: New Cloud & new Credential for that cloud
// Cluster B, User A: New Cloud & New Credential for that cloud (duplicate is necessary)
// Cluster_A, User_A: error. already exists.
// Cluster_A, User_B: No new Cloud, new Credential for the cloud.

type ClientConfigFunc func() (*ClientConfig, error)

func NewClientConfigReader(cloudType string) (ClientConfigFunc, error) {
	switch cloudType {
	case "kubernetes":
		return K8SClientConfig, nil
	default:
		return nil, errors.Errorf("Cannot read local config: unsupported cloud type '%s'", cloudType)
	}
}
