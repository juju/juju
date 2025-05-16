// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package maas

import (
	"fmt"
	stdtesting "testing"

	"github.com/juju/tc"
	goyaml "gopkg.in/yaml.v2"

	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/core/paths"
	"github.com/juju/juju/internal/cloudconfig/cloudinit"
)

type utilSuite struct{}

func TestUtilSuite(t *stdtesting.T) { tc.Run(t, &utilSuite{}) }
func (*utilSuite) TestExtractSystemId(c *tc.C) {
	instanceId := instance.Id("/MAAS/api/1.0/nodes/system_id/")

	systemId := extractSystemId(instanceId)

	c.Check(systemId, tc.Equals, "system_id")
}

func (*utilSuite) TestGetSystemIdValues(c *tc.C) {
	instanceId1 := instance.Id("/MAAS/api/1.0/nodes/system_id1/")
	instanceId2 := instance.Id("/MAAS/api/1.0/nodes/system_id2/")
	instanceIds := []instance.Id{instanceId1, instanceId2}

	values := getSystemIdValues("id", instanceIds)

	c.Check(values["id"], tc.DeepEquals, []string{"system_id1", "system_id2"})
}

func (*utilSuite) TestMachineInfoCloudinitRunCmd(c *tc.C) {
	hostname := "hostname"
	info := machineInfo{hostname}
	filename := "/var/lib/juju/MAASmachine.txt"
	dataDir := paths.DataDir(paths.OSUnixLike)
	cloudcfg, err := cloudinit.New("ubuntu")
	c.Assert(err, tc.ErrorIsNil)
	script, err := info.cloudinitRunCmd(cloudcfg)
	c.Assert(err, tc.ErrorIsNil)
	yaml, err := goyaml.Marshal(info)
	c.Assert(err, tc.ErrorIsNil)
	expected := fmt.Sprintf("mkdir -p '%s'\ncat > '%s' << 'EOF'\n'%s'\nEOF\nchmod 0755 '%s'", dataDir, filename, yaml, filename)
	c.Check(script, tc.Equals, expected)
}

func (*utilSuite) TestMachineInfoLoad(c *tc.C) {
	hostname := "hostname"
	yaml := fmt.Sprintf("hostname: %s\n", hostname)
	filename := createTempFile(c, []byte(yaml))
	old_MAASInstanceFilename := _MAASInstanceFilename
	_MAASInstanceFilename = filename
	defer func() { _MAASInstanceFilename = old_MAASInstanceFilename }()
	info := machineInfo{}

	err := info.load()

	c.Assert(err, tc.ErrorIsNil)
	c.Check(info.Hostname, tc.Equals, hostname)
}
