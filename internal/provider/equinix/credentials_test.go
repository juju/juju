// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package equinix

import (
	"os"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type credentialsSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (e credentialsSuite) TestDetectCredentials(c *gc.C) {
	cred := environProviderCredentials{}
	os.Setenv("METAL_AUTH_TOKEN", "tokenright")
	os.Setenv("METAL_PROJECT_ID", "project-id")
	_, err := cred.DetectCredentials("equinix_test")
	c.Assert(err, jc.ErrorIsNil)
}

func (e credentialsSuite) TestDetectCredentials_NoMetalToken(c *gc.C) {
	cred := environProviderCredentials{}
	os.Setenv("METAL_PROJECT_ID", "project-id")
	_, err := cred.DetectCredentials("equinix_test")
	c.Assert(err.Error(), jc.Contains, "equinix metal auth token not found")
}

func (e credentialsSuite) TestDetectCredentials_NoProject(c *gc.C) {
	cred := environProviderCredentials{}
	os.Setenv("METAL_AUTH_TOKEN", "metal")
	_, err := cred.DetectCredentials("equinix_test")
	c.Assert(err.Error(), jc.Contains, "equinix metal project ID not found")
}
