// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/charm/v8"
	charmresource "github.com/juju/charm/v8/resource"
	"github.com/juju/gnuflag"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apicommoncharms "github.com/juju/juju/api/common/charms"
	"github.com/juju/juju/cmd/juju/application/utils/mocks"
	"github.com/juju/juju/core/instance"
)

type utilsSuite struct {
}

var _ = gc.Suite(&utilsSuite{})

func (s *utilsSuite) TestParsePlacement(c *gc.C) {
	obtained, err := ParsePlacement("lxd:1")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(*obtained, jc.DeepEquals, instance.Placement{Scope: "lxd", Directive: "1"})

}

func (s *utilsSuite) TestGetFlags(c *gc.C) {
	flagSet := gnuflag.NewFlagSet("testing", gnuflag.ContinueOnError)
	flagSet.Bool("debug", true, "debug")
	flagSet.String("to", "", "to")
	flagSet.String("m", "default", "model")
	err := flagSet.Set("to", "lxd")
	c.Assert(err, jc.ErrorIsNil)
	obtained := GetFlags(flagSet, []string{"to", "force"})
	c.Assert(obtained, gc.DeepEquals, []string{"--to"})
}

type utilsResourceSuite struct {
	charmClient *mocks.MockCharmClient
}

var _ = gc.Suite(&utilsResourceSuite{})

func (s *utilsResourceSuite) TestGetMetaResources(c *gc.C) {
	defer s.setupMocks(c).Finish()

	curl := charm.MustParseURL("local:trusty/multi-series-1")
	s.expectCharmInfo(curl.String())

	obtained, err := GetMetaResources(curl, s.charmClient)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(obtained, gc.DeepEquals, map[string]charmresource.Meta{
		"test": {Name: "Testme"}})
}

func (s *utilsResourceSuite) TestGetUpgradeResources(c *gc.C) {
	// TODO:
	// Testing of GetUpgradeResources is well covered as part of
	// the bundle handler and upgrade charm testing.  The more
	// detailed configurations can be moved here.
	c.Skip("ImplementMe")
}

func (s *utilsResourceSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.charmClient = mocks.NewMockCharmClient(ctrl)
	return ctrl
}

func (s *utilsResourceSuite) expectCharmInfo(str string) {
	charmInfo := &apicommoncharms.CharmInfo{
		Meta: &charm.Meta{
			Resources: map[string]charmresource.Meta{
				"test": {Name: "Testme"},
			},
		},
	}
	s.charmClient.EXPECT().CharmInfo(str).Return(charmInfo, nil)
}
