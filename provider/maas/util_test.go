// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"

	goyaml "gopkg.in/yaml.v1"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
)

type utilSuite struct{}

var _ = gc.Suite(&utilSuite{})

func (*utilSuite) TestExtractSystemId(c *gc.C) {
	instanceId := instance.Id("/MAAS/api/1.0/nodes/system_id/")

	systemId := extractSystemId(instanceId)

	c.Check(systemId, gc.Equals, "system_id")
}

func (*utilSuite) TestGetSystemIdValues(c *gc.C) {
	instanceId1 := instance.Id("/MAAS/api/1.0/nodes/system_id1/")
	instanceId2 := instance.Id("/MAAS/api/1.0/nodes/system_id2/")
	instanceIds := []instance.Id{instanceId1, instanceId2}

	values := getSystemIdValues("id", instanceIds)

	c.Check(values["id"], gc.DeepEquals, []string{"system_id1", "system_id2"})
}

func (*utilSuite) TestMachineInfoCloudinitRunCmd(c *gc.C) {
	hostname := "hostname"
	info := machineInfo{hostname}
	filename := "/var/lib/juju/MAASmachine.txt"
	script, err := info.cloudinitRunCmd("quantal")
	c.Assert(err, gc.IsNil)
	yaml, err := goyaml.Marshal(info)
	c.Assert(err, gc.IsNil)
	expected := fmt.Sprintf("mkdir -p '%s'\ninstall -m 755 /dev/null '%s'\nprintf '%%s\\n' ''\"'\"'%s'\"'\"'' > '%s'", environs.DataDir, filename, yaml, filename)
	c.Check(script, gc.Equals, expected)
}

func (*utilSuite) TestMachineInfoLoad(c *gc.C) {
	hostname := "hostname"
	yaml := fmt.Sprintf("hostname: %s\n", hostname)
	filename := createTempFile(c, []byte(yaml))
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()
	info := machineInfo{}

	err := info.load()

	c.Assert(err, gc.IsNil)
	c.Check(info.Hostname, gc.Equals, hostname)
}
