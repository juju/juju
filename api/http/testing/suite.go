// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"encoding/base64"

	gc "gopkg.in/check.v1"

	apihttp "github.com/juju/juju/api/http"
	"github.com/juju/juju/testing"
)

// BaseSuite provides basic testing capability for API HTTP tests.
type BaseSuite struct {
	testing.BaseSuite
	// Fake is the fake HTTP client used in tests.
	Fake *FakeHTTPClient
}

func (s *BaseSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Fake = NewFakeHTTPClient()
}

func (s *BaseSuite) CheckRequest(c *gc.C, req *apihttp.Request, method, user, pw, host, pth string) {
	// Only check API-related request fields.

	c.Check(req.Method, gc.Equals, method)

	url := `https://` + host + `:\d+/environment/[-0-9a-f]+/` + pth
	c.Check(req.URL.String(), gc.Matches, url)

	c.Assert(req.Header, gc.HasLen, 1)
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pw))
	c.Check(req.Header.Get("Authorization"), gc.Equals, "Basic "+auth)
}
