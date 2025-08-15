// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	// Import shas that are used for docker image validation.
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/json"

	"github.com/distribution/reference"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/docker"
)

// DockerImageDetails holds the details for a Docker resource type.
type DockerImageDetails struct {
	// RegistryPath holds the path of the Docker image (including host and sha256) in a docker registry.
	RegistryPath string `json:"ImageName" yaml:"registrypath"`

	docker.ImageRepoDetails `json:",inline" yaml:",inline"`
}

// IsPrivate shows if the image repo is private or not.
func (did DockerImageDetails) IsPrivate() bool {
	return did.ImageRepoDetails.IsPrivate()
}

// ValidateDockerRegistryPath ensures the registry path is valid (i.e. api.jujucharms.com@sha256:deadbeef)
func ValidateDockerRegistryPath(path string) error {
	_, err := reference.ParseNormalizedNamed(path)
	if err != nil {
		return errors.NotValidf("docker image path %q", path)
	}
	return nil
}

// CheckDockerDetails validates the provided resource is suitable for use.
func CheckDockerDetails(name string, details DockerImageDetails) error {
	// TODO (veebers): Validate the URL actually works.
	return ValidateDockerRegistryPath(details.RegistryPath)
}

// UnmarshalDockerResource unmarshals the docker resource file from data.
func UnmarshalDockerResource(data []byte) (DockerImageDetails, error) {
	var resourceBody DockerImageDetails
	// Older clients sent the resources as a json string.
	err := json.Unmarshal(data, &resourceBody)
	if err != nil {
		if err := yaml.Unmarshal(data, &resourceBody); err != nil {
			return DockerImageDetails{}, errors.Annotate(err, "docker resource is neither valid json or yaml")
		}
	}
	return resourceBody, nil
}
