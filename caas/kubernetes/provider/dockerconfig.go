// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"encoding/json"
	"regexp"

	"github.com/juju/juju/caas"
)

// Used to extract the registry from an Image Path.
// i.e. registry.staging.jujucharms.com from registry.staging.jujucharms.com/tester/caas-mysql/mysql-image@sha256:dead-beef
var dockerURLRegexp = regexp.MustCompile(`^(?P<registryURL>([^.]+([.]+[^.\/]+)+))\/.*$`)

// These Docker Config datatypes have been pulled from
// "k8s.io/kubernetes/pkg/credentialprovider".
// multiple k8s packages import the same package, we don't yet have the tooling
// to flatten the deps.
// The specific package in this case is golog.

// DockerConfigJson represents ~/.docker/config.json file info.
type DockerConfigJson struct {
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

func createDockerConfigJSON(imageDetails *caas.ImageDetails) ([]byte, error) {
	dockerEntry := DockerConfigEntry{
		Username: imageDetails.Username,
		Password: imageDetails.Password,
	}
	registryURL := extractRegistryURL(imageDetails.ImagePath)

	dockerConfig := DockerConfigJson{
		Auths: map[string]DockerConfigEntry{
			registryURL: dockerEntry,
		},
	}
	return json.Marshal(dockerConfig)
}

// extractRegistryName returns the registry URL part of an images path
func extractRegistryURL(imagePath string) string {
	registryURL := "docker.io"
	hasRegistryURL := dockerURLRegexp.MatchString(imagePath)
	if hasRegistryURL {
		registryURL = dockerURLRegexp.ReplaceAllString(imagePath, "$registryURL")
	}
	return registryURL
}
