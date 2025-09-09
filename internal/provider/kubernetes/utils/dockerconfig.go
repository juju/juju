// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	// Import shas that are used for docker image validation.
	_ "crypto/sha256"
	_ "crypto/sha512"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/distribution/reference"
	"github.com/juju/errors"
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

func CreateDockerConfigJSON(username, password, imagePath string) ([]byte, error) {
	dockerEntry := DockerConfigEntry{
		Username: username,
		Password: password,
	}
	registryURL, err := ExtractRegistryURL(imagePath)
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

// ExtractRegistryName returns the registry URL part of an images path
func ExtractRegistryURL(imagePath string) (string, error) {
	imageNamed, err := reference.ParseNormalizedNamed(imagePath)
	if err != nil {
		return "", errors.Annotate(err, "extracting registry from path")
	}
	domain := reference.Domain(imageNamed)
	if domain == "docker.io" && !strings.HasPrefix(strings.ToLower(imagePath), "docker.io") {
		return "", fmt.Errorf("oci reference %q must have a domain", imagePath)
	}
	return domain, nil
}
