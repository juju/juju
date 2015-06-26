// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	goyaml "gopkg.in/yaml.v1"

	"github.com/juju/juju/cloudconfig/cloudinit"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/juju/paths"
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
	dataDir, err := paths.DataDir("quantal")
	c.Assert(err, jc.ErrorIsNil)
	cloudcfg, err := cloudinit.New("quantal")
	c.Assert(err, jc.ErrorIsNil)
	script, err := info.cloudinitRunCmd(cloudcfg)
	c.Assert(err, jc.ErrorIsNil)
	yaml, err := goyaml.Marshal(info)
	c.Assert(err, jc.ErrorIsNil)
	expected := fmt.Sprintf("mkdir -p '%s'\ncat > '%s' << 'EOF'\n'%s'\nEOF\nchmod 0755 '%s'", dataDir, filename, yaml, filename)
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

	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.Hostname, gc.Equals, hostname)
}
