// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"strings"

	"github.com/juju/errors"
	"github.com/juju/version/v2"

	"github.com/juju/juju/docker/registry/image"
	"github.com/juju/juju/tools"
)

type tagsResponseLayerV1 struct {
	Name string `json:"name"`
}

type tagsResponseV1 []tagsResponseLayerV1

func (r tagsResponseV1) GetTags() []string {
	var tags []string
	for _, v := range r {
		tags = append(tags, v.Name)
	}
	return tags
}

type tagsResponseV2 struct {
	Tags []string `json:"tags"`
}

func (r tagsResponseV2) GetTags() []string {
	return r.Tags
}

type tagsGetter interface {
	GetTags() []string
}

func getRepositoryOnly(s string) string {
	i := strings.IndexRune(s, '/')
	if i == -1 {
		return s
	}
	return s[i+1:]
}

// Tags fetches tags for an OCI image.
func (c baseClient) Tags(imageName string) (versions tools.Versions, err error) {
	apiVersion := c.APIVersion()

	repo := getRepositoryOnly(c.repoDetails.Repository)
	if apiVersion == APIVersionV1 {
		url := c.url("/repositories/%s/%s/tags", repo, imageName)
		var response tagsResponseV1
		return c.fetchTags(url, &response)
	}
	if apiVersion == APIVersionV2 {
		url := c.url("/%s/%s/tags/list", repo, imageName)
		var response tagsResponseV2
		return c.fetchTags(url, &response)
	}
	// This should never happen.
	return nil, nil
}

func (c baseClient) fetchTags(url string, res tagsGetter) (versions tools.Versions, err error) {
	pushVersions := func(tags []string) {
		for _, tag := range tags {
			v, err := version.Parse(tag)
			if err != nil {
				logger.Warningf("ignoring unexpected image tag %q", tag)
				continue
			}
			versions = append(versions, image.NewImageInfo(v))
		}
	}
	for {
		url, err = c.getPaginatedJSON(url, &res)
		switch err {
		case errNoMorePages:
			pushVersions(res.GetTags())
			return versions, nil
		case nil:
			pushVersions(res.GetTags())
			continue
		default:
			return nil, errors.Trace(err)
		}
	}
}
