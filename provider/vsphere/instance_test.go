// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package vsphere_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/provider/vsphere"
	"github.com/juju/juju/status"
)

type InstanceSuiteSuite struct {
	vsphere.BaseSuite
	namespace instance.Namespace
}

var _ = gc.Suite(&InstanceSuiteSuite{})

func (s *InstanceSuiteSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	namespace, err := instance.NewNamespace(s.Env.Config().UUID())
	c.Assert(err, jc.ErrorIsNil)
	s.namespace = namespace
}

func (s *InstanceSuiteSuite) machineName(c *gc.C, id string) string {
	name, err := s.namespace.Hostname(id)
	c.Assert(err, jc.ErrorIsNil)
	return name
}

func (s *InstanceSuiteSuite) TestInstances(c *gc.C) {
	client, closer, err := vsphere.ExposeEnvFakeClient(s.Env)
	c.Assert(err, jc.ErrorIsNil)
	defer closer()
	s.FakeClient = client
	client.SetPropertyProxyHandler("FakeDatacenter", vsphere.RetrieveDatacenterProperties)
	vmName1 := s.machineName(c, "1")
	vmName2 := s.machineName(c, "2")
	s.FakeInstances(client, vsphere.Inst{
		Inst:       vmName1,
		PowerState: "poweredOn",
	}, vsphere.Inst{
		Inst:       vmName2,
		PowerState: "poweredOff",
	})

	instances, err := s.Env.Instances([]instance.Id{
		instance.Id(vmName1),
		instance.Id(vmName2),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(instances[0].Status(), jc.DeepEquals, instance.InstanceStatus{
		Status:  status.Running,
		Message: "poweredOn",
	})
	c.Assert(instances[1].Status(), jc.DeepEquals, instance.InstanceStatus{
		Status:  status.Empty,
		Message: "poweredOff",
	})
}
