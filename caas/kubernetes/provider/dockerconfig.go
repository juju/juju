// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	// Import shas that are used for docker image validation.
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/json"

	"github.com/docker/distribution/reference"
	"github.com/juju/errors"

	"github.com/juju/juju/caas/specs"
)

// These Docker Config datatypes have been pulled from
// "k8s.io/kubernetes/pkg/credentialprovider".
// multiple k8s packages import the same package, we don't yet have the tooling
// to flatten the deps.
// The specific package in this case is golog.

// DockerConfigJSON represents ~/.docker/config.json file info.
type DockerConfigJSON struct {
	Auths DockerConfig `json:"auths"`
}

// DockerConfig represents the config file used by the docker CLI.
type DockerConfig map[string]DockerConfigEntry

// DockerConfigEntry represents an Auth entry in the dockerconfigjson.
type DockerConfigEntry struct {
	Username string
	Password string
	Email    string
}

func createDockerConfigJSON(imageDetails *specs.ImageDetails) ([]byte, error) {
	dockerEntry := DockerConfigEntry{
		Username: imageDetails.Username,
		Password: imageDetails.Password,
	}
	registryURL, err := extractRegistryURL(imageDetails.ImagePath)
	if err != nil {
		return nil, errors.Trace(err)
	}

	dockerConfig := DockerConfigJSON{
		Auths: map[string]DockerConfigEntry{
			registryURL: dockerEntry,
		},
	}
	return json.Marshal(dockerConfig)
}

// extractRegistryName returns the registry URL part of an images path
func extractRegistryURL(imagePath string) (string, error) {
	imageNamed, err := reference.ParseNormalizedNamed(imagePath)
	if err != nil {
		return "", errors.Annotate(err, "extracting registry from path")
	}
	return reference.Domain(imageNamed), nil
}
