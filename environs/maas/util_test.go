// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/goyaml"
	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
)

type UtilSuite struct{}

var _ = Suite(&UtilSuite{})

func (s *UtilSuite) TestExtractSystemId(c *C) {
	instanceId := instance.Id("/MAAS/api/1.0/nodes/system_id/")

	systemId := extractSystemId(instanceId)

	c.Check(systemId, Equals, "system_id")
}

func (s *UtilSuite) TestGetSystemIdValues(c *C) {
	instanceId1 := instance.Id("/MAAS/api/1.0/nodes/system_id1/")
	instanceId2 := instance.Id("/MAAS/api/1.0/nodes/system_id2/")
	instanceIds := []instance.Id{instanceId1, instanceId2}

	values := getSystemIdValues(instanceIds)

	c.Check(values["id"], DeepEquals, []string{"system_id1", "system_id2"})
}

func (s *UtilSuite) TestMachineInfoCloudinitRunCmd(c *C) {
	hostname := "hostname"
	filename := "path/to/file"
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()
	info := machineInfo{hostname}

	script, err := info.cloudinitRunCmd()

	c.Assert(err, IsNil)
	yaml, err := goyaml.Marshal(info)
	c.Assert(err, IsNil)
	expected := fmt.Sprintf("mkdir -p '%s'; echo -n '%s' > '%s'", environs.DataDir, yaml, filename)
	c.Check(script, Equals, expected)
}

func (s *UtilSuite) TestMachineInfoLoad(c *C) {
	hostname := "hostname"
	yaml := fmt.Sprintf("hostname: %s\n", hostname)
	filename := createTempFile(c, []byte(yaml))
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()
	info := machineInfo{}

	err := info.load()

	c.Assert(err, IsNil)
	c.Check(info.Hostname, Equals, hostname)
}
