// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/application"
	basetesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/bundle"
	coretesting "github.com/juju/juju/testing"
)

type bundleMockSuite struct {
	coretesting.BaseSuite
	charmsClient *bundle.Client
}

var _ = gc.Suite(&bundleMockSuite{})

func newClient(f basetesting.APICallerFunc) *application.Client {
	return application.NewClient(basetesting.BestVersionCaller{f, 2})
}

func (s *bundleMockSuite) TestExportBundle(c *gc.C) {

}
