// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caas

import (
	"fmt"

	"github.com/juju/errors"
	"gopkg.in/yaml.v2"
)

// ContainerPort defines the attributes used to configure
// an open port for the container.
type ContainerPort struct {
	ContainerPort int    `yaml:"container-port"`
	Protocol      string `yaml:"protocol"`
}

// FileSet defines a set of files to mount
// into the container.
type FileSet struct {
	Name      string            `yaml:"name"`
	MountPath string            `yaml:"mount-path"`
	Files     map[string]string `yaml:"files,omitempty"`
	Secret    *FileSecret       `yaml:"secret,omitempty"`
}

// FileSecret defines a secret to use add to a mount point
// for a container in the CAAS substrate.
type FileSecret struct {
	Name       string    `yaml:"name"`
	SecretKeys []KeyPath `yaml:"keys,omitempty"`
}

// KeyPath defines a key and file path.
type KeyPath struct {
	Key  string `yaml:"key"`
	Path string `yaml:"path"`
}

// ContainerSpec defines the data values used to configure
// a container on the CAAS substrate.
type ContainerSpec struct {
	Name          string                  `yaml:"name"`
	ImageName     string                  `yaml:"image-name"`
	Ports         []ContainerPort         `yaml:"ports,omitempty"`
	Files         []FileSet               `yaml:"files,omitempty"`
	Config        map[string]string       `yaml:"-"`
	ConfigSecrets map[string]ConfigSecret `yaml:"-"`
}

// ConfigSecret defines a secret to use when configuring a
// container in the CAAS substrate.
type ConfigSecret struct {
	SecretName string `yaml:"secret"`
	Key        string `yaml:"key"`
}

type containerYaml struct {
	ContainerSpec `yaml:",inline"`
	RawConfig     map[string]interface{} `yaml:"config,omitempty"`
}

// ParseContainerSpec parses a YAML string into a ContainerSpec struct.
func ParseContainerSpec(in string) (*ContainerSpec, error) {
	var rawSpec containerYaml
	if err := yaml.Unmarshal([]byte(in), &rawSpec); err != nil {
		return nil, errors.Trace(err)
	}
	if rawSpec.Name == "" {
		return nil, errors.New("spec name is missing")
	}
	if rawSpec.ImageName == "" {
		return nil, errors.New("spec image name is missing")
	}
	for _, fs := range rawSpec.Files {
		if fs.Name == "" {
			return nil, errors.New("file set name is missing")
		}
		if fs.MountPath == "" {
			return nil, errors.Errorf("mount path is missing for file set %q", fs.Name)
		}
	}
	for k, v := range rawSpec.RawConfig {
		switch val := v.(type) {
		case string:
			if rawSpec.Config == nil {
				rawSpec.Config = make(map[string]string)
			}
			rawSpec.Config[k] = val
		case map[interface{}]interface{}:
			if count := len(val); count != 2 {
				return nil, errors.Errorf("expected 2 values in secret spec for %q, got %d", k, count)
			}
			secret, ok := val["secret"]
			if !ok {
				return nil, errors.Errorf("missing secret name for secret %q", k)
			}
			key, ok := val["key"]
			if !ok {
				return nil, errors.Errorf("missing key for secret %q", k)
			}
			if rawSpec.ConfigSecrets == nil {
				rawSpec.ConfigSecrets = make(map[string]ConfigSecret)
			}
			rawSpec.ConfigSecrets[k] = ConfigSecret{
				SecretName: fmt.Sprintf("%v", secret),
				Key:        fmt.Sprintf("%v", key),
			}
		default:
			return nil, errors.Errorf("unexpected config value type %T", val)
		}
	}
	return &rawSpec.ContainerSpec, nil
}
