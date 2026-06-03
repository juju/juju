// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/url"

	gc "gopkg.in/check.v1"
)

type commonURLGetterSuite struct{}

var _ = gc.Suite(&commonURLGetterSuite{})

func (s *commonURLGetterSuite) TestRootPathKeepsTrailingSlash(c *gc.C) {
	baseURL := url.URL{Scheme: "https", Host: "ghcr.io"}
	got := commonURLGetter(APIVersionV2, baseURL, "/")
	c.Assert(got, gc.Equals, "https://ghcr.io/v2/")
}

func (s *commonURLGetterSuite) TestPathWithLeadingVersionPrefix(c *gc.C) {
	baseURL := url.URL{Scheme: "https", Host: "ghcr.io"}

	got := commonURLGetter(
		APIVersionV2,
		baseURL,
		"/v2/jujuqa/jujud-operator/tags/list",
	)
	c.Assert(got, gc.Equals, "https://ghcr.io/v2/jujuqa/jujud-operator/tags/list")
}

func (s *commonURLGetterSuite) TestPathWithVersionPrefixNoLeadingSlash(c *gc.C) {
	baseURL := url.URL{Scheme: "https", Host: "ghcr.io"}

	got := commonURLGetter(
		APIVersionV2,
		baseURL,
		"v2/jujuqa/jujud-operator/tags/list",
	)
	c.Assert(got, gc.Equals, "https://ghcr.io/v2/jujuqa/jujud-operator/tags/list")
}
