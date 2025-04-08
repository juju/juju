// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"encoding/json"

	"github.com/distribution/reference"
	"github.com/juju/errors"
	"gopkg.in/yaml.v3"

	"github.com/juju/juju/core/resource"
)

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

// ConvertToResourceImageDetails converts the provided DockerImageDetails to a
// resources.ImageRepoDetails.
func ConvertToResourceImageDetails(imageRepo ImageRepoDetails) resource.ImageRepoDetails {
	return resource.ImageRepoDetails{
		BasicAuthConfig: resource.BasicAuthConfig{
			Auth:     convertToResourceToken(imageRepo.BasicAuthConfig.Auth),
			Username: imageRepo.BasicAuthConfig.Username,
			Password: imageRepo.BasicAuthConfig.Password,
		},

		TokenAuthConfig: resource.TokenAuthConfig{
			IdentityToken: convertToResourceToken(imageRepo.TokenAuthConfig.IdentityToken),
			RegistryToken: convertToResourceToken(imageRepo.TokenAuthConfig.RegistryToken),
		},
		Repository:    imageRepo.Repository,
		ServerAddress: imageRepo.ServerAddress,
		Region:        imageRepo.Region,
	}
}

func convertToResourceToken(t *Token) *resource.Token {
	if t == nil {
		return nil
	}
	return &resource.Token{
		Value:     t.Value,
		ExpiresAt: t.ExpiresAt,
	}
}
