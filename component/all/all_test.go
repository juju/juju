// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The all package facilitates the registration of Juju components into
// the relevant machinery. It is intended as the one place in Juju where
// the components (horizontal design layers) and the machinery
// (vertical/architectural layers) intersect. This approach helps
// alleviate interdependence between the components and the machinery.

package all

import (
	"testing"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	jujutesting "github.com/juju/juju/testing"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type allSuite struct {
	jujutesting.BaseSuite
}

var _ = gc.Suite(&allSuite{})

func (s *allSuite) TestRegisterForContainerAgent(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	resources := NewMockComponent(ctrl)
	components := []Component{
		resources,
	}

	gomock.InOrder(
		resources.EXPECT().registerForContainerAgent().Return(nil),
	)

	err := registerForContainerAgent(components)
	c.Assert(err, jc.ErrorIsNil)
}
