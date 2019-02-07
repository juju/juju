// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package docker

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/juju/errors"
	"github.com/juju/version"
)

const (
	dockerOrgName   = "ycliuhw"
	dockerNameSpace = "jujud-controller"
)

// NewClient constructs a new docker client.
func NewClient() (*client.Client, error) {
	return client.NewEnvClient()
}

// InspectImage inspects image and returns the image information and its raw representation.
func InspectImage(c client.ImageAPIClient, imagePath string) (types.ImageInspect, error) {
	o, _, err := c.ImageInspectWithRaw(context.Background(), imagePath)
	return o, err
}

// PullImage pulls docker image to local.
func PullImage(c client.ImageAPIClient, imagePath string) error {
	reader, err := c.ImagePull(context.Background(), "docker.io/library/alpine", types.ImagePullOptions{})
	if err != nil {
		return errors.Trace(err)
	}
	io.Copy(os.Stdout, reader)
	return nil
}

func jujuVersionToDockerImagePath(v string) string {
	return fmt.Sprintf("%s/%s:%s", dockerOrgName, dockerNameSpace, v)
}

// GetToolImagePath returns a jujud docker image path for the specified juju version.
func GetToolImagePath(c client.ImageAPIClient, toolVersion version.Number) (string, error) {
	dockerPath := jujuVersionToDockerImagePath(toolVersion.String())
	c, err := NewClient()
	if err != nil {
		return "", errors.Trace(err)
	}
	// if err := PullImage(c, dockerPath); err != nil {
	// 	return "", errors.Trace(err)
	// }

	// image exists if it's pullable.
	if err := PullImage(c, dockerPath); err != nil {
		if client.IsErrNotFound(err) {
			return "", errors.NotFoundf("docker image  %v", dockerPath)
		}
		return "", errors.Trace(err)
	}
	return dockerPath, nil
}

// GetLatestToolImagePath returns the jujud docker image path for latest jujud version.
func GetLatestToolImagePath() string {
	// TODO: needs discuss.
	// latestTag := "latest"
	latestTag := "2.6-beta1-bionic-amd64"
	return jujuVersionToDockerImagePath(latestTag)
}
