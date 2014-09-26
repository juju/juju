// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"strings"
	"time"

	"github.com/Altoros/gosigma"
	"github.com/Altoros/gosigma/data"
	"github.com/Altoros/gosigma/mock"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/cloudinit"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
	"github.com/juju/juju/version"
	"github.com/juju/loggo"
	gc "gopkg.in/check.v1"
)

type clientSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	mock.Start()
}

func (s *clientSuite) TearDownSuite(c *gc.C) {
	mock.Stop()
	s.BaseSuite.TearDownSuite(c)
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	ll := logger.LogLevel()
	logger.SetLogLevel(loggo.TRACE)
	s.AddCleanup(func(*gc.C) { logger.SetLogLevel(ll) })

	mock.Reset()
}

func (s *clientSuite) TearDownTest(c *gc.C) {
	mock.Reset()
	s.BaseSuite.TearDownTest(c)
}

func testNewClient(c *gc.C, endpoint, username, password string) (*environClient, error) {
	ecfg := &environConfig{
		Config: newConfig(c, testing.Attrs{"name": "client-test", "uuid": "f54aac3a-9dcd-4a0c-86b5-24091478478c"}),
		attrs: map[string]interface{}{
			"region":   endpoint,
			"username": username,
			"password": password,
		},
	}
	return newClient(ecfg)
}

func (s *clientSuite) TestClientNew(c *gc.C) {
	cli, err := testNewClient(c, "https://testing.invalid", "user", "password")
	c.Check(err, gc.IsNil)
	c.Check(cli, gc.NotNil)

	cli, err = testNewClient(c, "http://testing.invalid", "user", "password")
	c.Check(err, gc.ErrorMatches, "endpoint must use https scheme")
	c.Check(cli, gc.IsNil)

	cli, err = testNewClient(c, "https://testing.invalid", "", "password")
	c.Check(err, gc.ErrorMatches, "username is not allowed to be empty")
	c.Check(cli, gc.IsNil)
}

func addTestClientServer(c *gc.C, instance, env, ip string) string {
	json := `{"meta": {`
	if instance != "" {
		json += fmt.Sprintf(`"juju-instance": "%s"`, instance)
		if env != "" {
			json += fmt.Sprintf(`, "juju-environment": "%s"`, env)
		}
	}
	json += fmt.Sprintf(`}, "status": "running", "nics":[{
            "runtime": {
                "interface_type": "public",
                "ip_v4": {
                    "resource_uri": "/api/2.0/ips/%s/",
                    "uuid": "%s"
                }}}]}`, ip, ip)
	r := strings.NewReader(json)
	s, err := data.ReadServer(r)
	c.Assert(err, gc.IsNil)
	mock.AddServer(s)
	return s.UUID
}

func (s *clientSuite) TestClientInstances(c *gc.C) {
	addTestClientServer(c, "", "", "")
	addTestClientServer(c, jujuMetaInstanceServer, "alien", "")
	addTestClientServer(c, jujuMetaInstanceStateServer, "alien", "")
	addTestClientServer(c, jujuMetaInstanceServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c", "1.1.1.1")
	addTestClientServer(c, jujuMetaInstanceServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c", "2.2.2.2")
	suuid := addTestClientServer(c, jujuMetaInstanceStateServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c", "3.3.3.3")

	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	ss, err := cli.instances()
	c.Assert(err, gc.IsNil)
	c.Assert(ss, gc.NotNil)
	c.Check(ss, gc.HasLen, 3)

	sm, err := cli.instanceMap()
	c.Assert(err, gc.IsNil)
	c.Assert(sm, gc.NotNil)
	c.Check(sm, gc.HasLen, 3)

	uuid, ip, rc := cli.stateServerAddress()
	c.Check(uuid, gc.Equals, suuid)
	c.Check(ip, gc.Equals, "3.3.3.3")
	c.Check(rc, gc.Equals, true)
}

func (s *clientSuite) TestClientStopStateInstance(c *gc.C) {
	addTestClientServer(c, "", "", "")
	addTestClientServer(c, jujuMetaInstanceServer, "alien", "")
	addTestClientServer(c, jujuMetaInstanceStateServer, "alien", "")
	addTestClientServer(c, jujuMetaInstanceServer, "client-test", "1.1.1.1")
	addTestClientServer(c, jujuMetaInstanceServer, "client-test", "2.2.2.2")
	suuid := addTestClientServer(c, jujuMetaInstanceStateServer, "client-test", "3.3.3.3")

	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	cli.storage = &environStorage{uuid: suuid, tmp: true}

	err = cli.stopInstance(instance.Id(suuid))
	c.Assert(err, gc.IsNil)

	uuid, ip, rc := cli.stateServerAddress()
	c.Check(uuid, gc.Equals, "")
	c.Check(ip, gc.Equals, "")
	c.Check(rc, gc.Equals, false)
}

func (s *clientSuite) TestClientInvalidStopInstance(c *gc.C) {
	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	var id instance.Id
	err = cli.stopInstance(id)
	c.Check(err, gc.ErrorMatches, "invalid instance id")

	err = cli.stopInstance("1234")
	c.Check(err, gc.ErrorMatches, "404 Not Found.*")
}

func (s *clientSuite) TestClientInvalidServer(c *gc.C) {
	cli, err := testNewClient(c, "https://testing.invalid", mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	cli.conn.ConnectTimeout(10 * time.Millisecond)

	err = cli.stopInstance("1234")
	c.Check(err, gc.ErrorMatches, "broken connection")

	_, err = cli.instanceMap()
	c.Check(err, gc.ErrorMatches, "broken connection")

	uuid, ip, ok := cli.stateServerAddress()
	c.Check(uuid, gc.Equals, "")
	c.Check(ip, gc.Equals, "")
	c.Check(ok, gc.Equals, false)
}

func (s *clientSuite) TestClientNewInstanceInvalidParams(c *gc.C) {
	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	params := environs.StartInstanceParams{
		Constraints: constraints.Value{},
	}
	img := &imagemetadata.ImageMetadata{
		Id: validImageId,
	}
	server, drive, err := cli.newInstance(params, img)
	c.Check(server, gc.IsNil)
	c.Check(drive, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid configuration for new instance")
}

func (s *clientSuite) TestClientNewInstanceInvalidTemplate(c *gc.C) {
	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	params := environs.StartInstanceParams{
		Constraints: constraints.Value{},
		MachineConfig: &cloudinit.MachineConfig{
			Bootstrap: true,
			Tools: &tools.Tools{
				Version: version.Binary{
					Series: "trusty",
				},
			},
		},
	}
	img := &imagemetadata.ImageMetadata{
		Id: "invalid-id",
	}
	server, drive, err := cli.newInstance(params, img)
	c.Check(server, gc.IsNil)
	c.Check(drive, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "query drive template: 404 Not Found.*")
}

func (s *clientSuite) TestClientNewInstance(c *gc.C) {
	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	cli.conn.OperationTimeout(1 * time.Second)

	params := environs.StartInstanceParams{
		Constraints: constraints.Value{},
		MachineConfig: &cloudinit.MachineConfig{
			Bootstrap: true,
			Tools: &tools.Tools{
				Version: version.Binary{
					Series: "trusty",
				},
			},
		},
	}
	img := &imagemetadata.ImageMetadata{
		Id: validImageId,
	}
	cs, err := newConstraints(params.MachineConfig.Bootstrap,
		params.Constraints, img)
	c.Assert(cs, gc.NotNil)
	c.Check(err, gc.IsNil)

	templateDrive := &data.Drive{
		Resource: data.Resource{URI: "uri", UUID: cs.driveTemplate},
		LibraryDrive: data.LibraryDrive{
			Arch:      "arch",
			ImageType: "image-type",
			OS:        "os",
			Paid:      true,
		},
		Size:   2200 * gosigma.Megabyte,
		Status: "unmounted",
	}
	mock.ResetDrives()
	mock.LibDrives.Add(templateDrive)

	server, drive, err := cli.newInstance(params, img)
	c.Check(server, gc.NotNil)
	c.Check(drive, gc.NotNil)
	c.Check(err, gc.IsNil)
}
