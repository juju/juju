// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"context"
	"strings"

	"github.com/juju/errors"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry/image"
	"github.com/juju/juju/internal/tools"
)

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

type tagFetcher interface {
	url(string, ...interface{}) string
	fetchTags(string, tagsGetter) (tools.Versions, error)
	ImageRepoDetails() docker.ImageRepoDetails
}

func fetchTagsV2(c tagFetcher, imageName string) (tools.Versions, error) {
	repo := getRepositoryOnly(c.ImageRepoDetails().Repository)
	url := c.url("/%s/%s/tags/list", repo, imageName)
	var response tagsResponseV2
	return c.fetchTags(url, &response)
}

// Tags fetches tags for an OCI image.
func (c *baseClient) Tags(imageName string) (tools.Versions, error) {
	switch c.APIVersion() {
	case APIVersionV2:
		return fetchTagsV2(c, imageName)
	default:
		return nil, errors.NotSupportedf("registry API %q", c.APIVersion())
	}
}

func (c *baseClient) fetchTags(url string, res tagsGetter) (versions tools.Versions, err error) {
	pushVersions := func(tags []string) {
		for _, tag := range tags {
			v, err := semversion.Parse(tag)
			if err != nil {
				logger.Warningf(context.TODO(), "ignoring invalid image tag %q", tag)
				continue
			}
			versions = append(versions, image.NewImageInfo(v))
		}
	}
	for {
		logger.Tracef(context.TODO(), "fetching tags %q", url)
		url, err = c.getPaginatedJSON(url, &res)
		logger.Tracef(context.TODO(), "response %#v, err %v", res, err)
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
