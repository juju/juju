// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiaddressupdater

import (
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api/agent/caasagent"
	"github.com/juju/juju/api/agent/uniter"
	basemocks "github.com/juju/juju/api/base/mocks"
)

func TestAPIAddresserSuite(t *testing.T) {
	tc.Run(t, &apiAddresserSuite{})
}

type apiAddresserSuite struct{}

func (s *apiAddresserSuite) TestControllerAgentUsesCAASAgentV3(c *tc.C) {
	addresser, err := newAPIAddresser(names.NewControllerAgentTag("0"), newAPICaller(c))

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresser, tc.FitsTypeOf, &caasagent.APIAddressClient{})
}

func (s *apiAddresserSuite) TestUnitUsesUniter(c *tc.C) {
	addresser, err := newAPIAddresser(names.NewUnitTag("controller/0"), newAPICaller(c))

	c.Assert(err, tc.ErrorIsNil)
	c.Check(addresser, tc.FitsTypeOf, &uniter.Client{})
}

func newAPICaller(c *tc.C) *basemocks.MockAPICaller {
	ctrl := gomock.NewController(c)
	c.Cleanup(ctrl.Finish)
	caller := basemocks.NewMockAPICaller(ctrl)
	caller.EXPECT().BestFacadeVersion(gomock.Any()).Return(1)
	return caller
}
