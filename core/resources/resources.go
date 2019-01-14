// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	// Import shas that are used for docker image validation.
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/docker/distribution/reference"
	"github.com/juju/errors"
)

// DockerImageDetails holds the details for a Docker resource type.
type DockerImageDetails struct {
	// RegistryPath holds the path of the Docker image (including host and sha256) in a docker registry.
	RegistryPath string `json:"ImageName" yaml:"registrypath"`

	// Username holds the username used to gain access to a non-public image.
	Username string `json:"Username" yaml:"username"`

	// Password holds the password used to gain access to a non-public image.
	Password string `json:"Password,omitempty" yaml:"password"`
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
