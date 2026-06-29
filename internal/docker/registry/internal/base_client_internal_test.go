// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package internal

import (
	"net/url"
	"testing"

	"github.com/juju/tc"
)

func TestBaseSuite(t *testing.T) {
	tc.Run(t, &commonURLGetterSuite{})
}

type commonURLGetterSuite struct{}

func (s *commonURLGetterSuite) TestRootPathKeepsTrailingSlash(c *tc.C) {
	baseURL := url.URL{Scheme: "https", Host: "ghcr.io"}
	got := commonURLGetter(APIVersionV2, baseURL, "/")
	c.Assert(got, tc.Equals, "https://ghcr.io/v2/")
}

func (s *commonURLGetterSuite) TestPathWithLeadingVersionPrefix(c *tc.C) {
	baseURL := url.URL{Scheme: "https", Host: "ghcr.io"}

	got := commonURLGetter(
		APIVersionV2,
		baseURL,
		"/v2/jujuqa/jujud-operator/tags/list",
	)
	c.Assert(got, tc.Equals, "https://ghcr.io/v2/jujuqa/jujud-operator/tags/list")
}

func (s *commonURLGetterSuite) TestPathWithVersionPrefixNoLeadingSlash(c *tc.C) {
	baseURL := url.URL{Scheme: "https", Host: "ghcr.io"}

	got := commonURLGetter(
		APIVersionV2,
		baseURL,
		"v2/jujuqa/jujud-operator/tags/list",
	)
	c.Assert(got, tc.Equals, "https://ghcr.io/v2/jujuqa/jujud-operator/tags/list")
}
