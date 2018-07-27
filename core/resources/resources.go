// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	"regexp"

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

var (
	validDockerImageRegExp = regexp.MustCompile(`^([A-Za-z\.]+/)?(([A-Za-z-_\.])+/?)+((@sha256){0,1}:[A-Za-z0-9-_\.]+)?$`)
	registryPathSplit      = regexp.MustCompile(`^(?P<registryURL>([^.]+([.]+[^.\/]+)+))\/(?P<image>.*)$`)
)

// ValidateDockerRegistryPath ensures the registry path is valid (i.e. api.jujucharms.com@sha256:deadbeef)
func ValidateDockerRegistryPath(path string) error {
	if ok := validDockerImageRegExp.MatchString(path); !ok {
		return errors.NotValidf("docker image path %q", path)
	}
	return nil
}

// CheckDockerDetails validates the provided resource is suitable for use.
func CheckDockerDetails(name, registryPath string) error {
	// TODO (veebers): Validate the URL actually works.
	return ValidateDockerRegistryPath(registryPath)
}

// SplitRegistryPath extracts the repo and the image parts of a registry path.
func SplitRegistryPath(registryPath string) (repo string, image string, err error) {
	err = ValidateDockerRegistryPath(registryPath)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	easySplit := registryPathSplit.MatchString(registryPath)
	if easySplit {
		repo = registryPathSplit.ReplaceAllString(registryPath, "$registryURL")
		image = registryPathSplit.ReplaceAllString(registryPath, "$image")
		return repo, image, nil
	}

	return "", registryPath, nil
}
