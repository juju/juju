// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/juju/errors"
)

//go:generate go run go.uber.org/mock/mockgen -package mocks -destination mocks/manifests_mock.go github.com/juju/juju/internal/docker/registry/internal ArchitectureGetter

type manifestsResponseV1 struct {
	SchemaVersion int    `json:"schemaVersion"`
	Architecture  string `json:"architecture"`
}

type manifestsResponseV2Config struct {
	Digest string `json:"digest"`
}

type manifestsResponseV2 struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Config        manifestsResponseV2Config `json:"config"`
}

// ManifestsResult is the result of GetManifests.
type ManifestsResult struct {
	Architecture string
	Digest       string
}

// BlobsResponse is the result of GetBlobs.
type BlobsResponse struct {
	Architecture string `json:"architecture"`
}

// ArchitectureGetter defines manifests and blob APIs.
type ArchitectureGetter interface {
	GetManifests(imageName, tag string) (*ManifestsResult, error)
	GetBlobs(imageName, digest string) (*BlobsResponse, error)
}

// GetArchitecture returns the archtecture of the image for the specified tag.
func (c baseClient) GetArchitecture(imageName, tag string) (string, error) {
	return getArchitecture(imageName, tag, c)
}

func getArchitecture(imageName, tag string, client ArchitectureGetter) (string, error) {
	manifests, err := client.GetManifests(imageName, tag)
	if err != nil {
		return "", errors.Annotatef(err, "can not get manifests for %s:%s", imageName, tag)
	}
	if manifests.Architecture == "" && manifests.Digest == "" {
		return "", errors.New(fmt.Sprintf("faild to get manifests for %q %q", imageName, tag))
	}
	if manifests.Architecture != "" {
		return manifests.Architecture, nil
	}
	blobs, err := client.GetBlobs(imageName, manifests.Digest)
	if err != nil {
		return "", errors.Trace(err)
	}
	return blobs.Architecture, nil
}

// GetManifests returns the manifests of the image for the specified tag.
func (c baseClient) GetManifests(imageName, tag string) (*ManifestsResult, error) {
	repo := getRepositoryOnly(c.ImageRepoDetails().Repository)
	url := c.url("/%s/%s/manifests/%s", repo, imageName, tag)
	return c.GetManifestsCommon(url)
}

// GetManifestsCommon returns manifests result for the provided url.
func (c baseClient) GetManifestsCommon(url string) (*ManifestsResult, error) {
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, errors.Trace(unwrapNetError(err))
	}
	defer resp.Body.Close()
	return processManifestsResponse(resp)
}

const (
	manifestContentTypeV1 = "application/vnd.docker.distribution.manifest.v1"
	manifestContentTypeV2 = "application/vnd.docker.distribution.manifest.v2"
)

func processManifestsResponse(resp *http.Response) (*ManifestsResult, error) {
	contentTypes := resp.Header[http.CanonicalHeaderKey("Content-Type")]
	if len(contentTypes) == 0 {
		return nil, errors.NotSupportedf(`no "Content-Type" found in header of registry API response`)
	}
	contentType := contentTypes[0]
	notSupportedAPIVersionError := errors.NotSupportedf("manifest response version %q", contentType)
	parts := strings.Split(contentType, "+")
	if len(parts) != 2 {
		return nil, notSupportedAPIVersionError
	}
	switch parts[0] {
	case manifestContentTypeV1:
		var data manifestsResponseV1
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, errors.Trace(err)
		}
		return &ManifestsResult{Architecture: data.Architecture}, nil
	case manifestContentTypeV2:
		var data manifestsResponseV2
		if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
			return nil, errors.Trace(err)
		}
		return &ManifestsResult{Digest: data.Config.Digest}, nil
	default:
		return nil, notSupportedAPIVersionError
	}
}

// GetBlobs gets the archtecture of the image for the specified tag via blobs API.
func (c baseClient) GetBlobs(imageName, digest string) (*BlobsResponse, error) {
	repo := getRepositoryOnly(c.ImageRepoDetails().Repository)
	url := c.url("/%s/%s/blobs/%s", repo, imageName, digest)
	return c.GetBlobsCommon(url)
}

// GetBlobsCommon returns blobs result for the provided url.
func (c baseClient) GetBlobsCommon(url string) (*BlobsResponse, error) {
	resp, err := c.client.Get(url)
	logger.Tracef("getting blobs for %q, err %v", url, err)
	if err != nil {
		return nil, errors.Trace(unwrapNetError(err))
	}
	defer resp.Body.Close()
	var result BlobsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Trace(err)
	}
	return &result, nil
}
