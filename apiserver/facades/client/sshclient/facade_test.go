// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	stdtesting "testing"

	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/facades/client/sshclient"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/virtualhostname"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type facadeSuite struct{}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) newFacade(c *tc.C, modelType model.ModelType) *sshclient.Facade {
	adminTag := names.NewUserTag("admin")
	auth := apiservertesting.FakeAuthorizer{Tag: adminTag, AdminTag: adminTag}
	facade, err := sshclient.InternalFacade(
		coretesting.ControllerTag, coretesting.ModelTag, modelType,
		nil, nil, nil, nil, nil, nil, auth,
	)
	c.Assert(err, tc.ErrorIsNil)
	return facade
}

func (s *facadeSuite) TestVirtualHostnameRejectsMachinesForCAAS(c *tc.C) {
	facade := s.newFacade(c, model.CAAS)
	for _, machineID := range []string{"0", "0/lxd/1"} {
		result, err := facade.VirtualHostname(c.Context(), params.VirtualHostnameTargetArg{
			Tag: names.NewMachineTag(machineID).String(),
		})
		c.Check(err, tc.ErrorMatches,
			`cannot SSH to machine "[^"]+" in a Kubernetes model; specify a unit name, such as <app>/0`)
		c.Assert(result.Error, tc.NotNil)
		c.Check(result.Error.Message, tc.Equals, err.Error())
		c.Check(result.Address, tc.Equals, "")
	}
}

func (s *facadeSuite) TestVirtualHostnameMachineForIAAS(c *tc.C) {
	facade := s.newFacade(c, model.IAAS)
	result, err := facade.VirtualHostname(c.Context(), params.VirtualHostnameTargetArg{
		Tag: names.NewMachineTag("0").String(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.IsNil)
	info, err := virtualhostname.NewInfoMachineTarget(coretesting.ModelTag.Id(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Address, tc.Equals, info.String())
}

func (s *facadeSuite) TestVirtualHostnameUnitForCAAS(c *tc.C) {
	facade := s.newFacade(c, model.CAAS)
	container := "charm"
	result, err := facade.VirtualHostname(c.Context(), params.VirtualHostnameTargetArg{
		Tag: names.NewUnitTag("redis/0").String(), Container: &container,
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Error, tc.IsNil)
	info, err := virtualhostname.NewInfoContainerTarget(coretesting.ModelTag.Id(), "redis/0", container)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result.Address, tc.Equals, info.String())
}
