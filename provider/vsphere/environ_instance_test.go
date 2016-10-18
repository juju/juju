// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !gccgo

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/vsphere"
)

type environInstanceSuite struct {
	vsphere.BaseSuite
	namespace instance.Namespace
}

var _ = gc.Suite(&environInstanceSuite{})

func (s *environInstanceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	namespace, err := instance.NewNamespace(s.Env.Config().UUID())
	c.Assert(err, jc.ErrorIsNil)
	s.namespace = namespace
}

func (s *environInstanceSuite) machineName(c *gc.C, id string) string {
	name, err := s.namespace.Hostname(id)
	c.Assert(err, jc.ErrorIsNil)
	return name
}

func (s *environInstanceSuite) TestInstances(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	vmName1 := s.machineName(c, "1")
	vmName2 := s.machineName(c, "2")
	s.FakeInstancesWithResourcePool(client, vsphere.InstRp{Inst: vmName1, Rp: "rp1"}, vsphere.InstRp{Inst: vmName2, Rp: "rp2"})

	instances, err := s.Env.Instances([]instance.Id{instance.Id(vmName1), instance.Id(vmName2)})

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(len(instances), gc.Equals, 2)
	c.Assert(string(instances[0].Id()), gc.Equals, vmName1)
	c.Assert(string(instances[1].Id()), gc.Equals, vmName2)
}

func (s *environInstanceSuite) TestInstancesReturnNoInstances(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	s.FakeInstancesWithResourcePool(client, vsphere.InstRp{Inst: "Some name that don't match naming convention", Rp: "rp1"})

	_, err := s.Env.Instances([]instance.Id{instance.Id("Some other name")})

	c.Assert(err, gc.Equals, environs.ErrNoInstances)
}

func (s *environInstanceSuite) TestInstancesReturnPartialInstances(c *gc.C) {
	client := vsphere.ExposeEnvFakeClient(s.Env)
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	vmName1 := s.machineName(c, "1")
	vmName2 := s.machineName(c, "2")
	s.FakeInstancesWithResourcePool(client, vsphere.InstRp{Inst: vmName1, Rp: "rp1"}, vsphere.InstRp{Inst: "Some inst", Rp: "rp2"})

	_, err := s.Env.Instances([]instance.Id{instance.Id(vmName1), instance.Id(vmName2)})

	c.Assert(err, gc.Equals, environs.ErrPartialInstances)
}
