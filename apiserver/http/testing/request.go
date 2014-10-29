// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"encoding/base64"
	"net/http"

	gc "gopkg.in/check.v1"
)

// CheckRequest verifies that the HTTP request matches the args
// as an API request should.  We only check API-related request fields.
func CheckRequest(c *gc.C, req *http.Request, method, user, pw, host, pth string) {
	c.Check(req.Method, gc.Equals, method)

	url := `https://` + host + `:\d+/environment/[-0-9a-f]+/` + pth
	c.Check(req.URL.String(), gc.Matches, url)

	c.Assert(req.Header, gc.HasLen, 1)
	auth := base64.StdEncoding.EncodeToString([]byte(user + ":" + pw))
	c.Check(req.Header.Get("Authorization"), gc.Equals, "Basic "+auth)
}
