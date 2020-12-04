// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudsigma

import (
	"fmt"
	"strings"
	"time"

	"github.com/altoros/gosigma"
	"github.com/altoros/gosigma/data"
	"github.com/altoros/gosigma/mock"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v2"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/cloudconfig/instancecfg"
	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/environs"
	environscloudspec "github.com/juju/juju/environs/cloudspec"
	"github.com/juju/juju/environs/imagemetadata"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/tools"
)

type clientSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&clientSuite{})

func (s *clientSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
	mock.Start()
	s.AddCleanup(func(*gc.C) {
		mock.Stop()
	})
}

func (s *clientSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	ll := logger.LogLevel()
	logger.SetLogLevel(loggo.TRACE)
	s.AddCleanup(func(*gc.C) { logger.SetLogLevel(ll) })

	mock.Reset()
}

func (s *clientSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func testNewClient(c *gc.C, endpoint, username, password string) (*environClient, error) {
	cred := cloud.NewCredential(cloud.UserPassAuthType, map[string]string{
		"username": username,
		"password": password,
	})
	spec := environscloudspec.CloudSpec{
		Region:     "testregion",
		Endpoint:   endpoint,
		Credential: &cred,
	}
	return newClient(spec, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
}

func addTestClientServer(c *gc.C, instance, env string) string {
	json := `{"meta": {`
	if instance != "" {
		json += fmt.Sprintf(`"juju-instance": "%s"`, instance)
		if env != "" {
			json += fmt.Sprintf(`, "juju-model": "%s"`, env)
		}
	}
	json += `}, "status": "running"}`
	r := strings.NewReader(json)
	s, err := data.ReadServer(r)
	c.Assert(err, gc.IsNil)
	mock.AddServer(s)
	return s.UUID
}

func (s *clientSuite) TestClientInstances(c *gc.C) {
	addTestClientServer(c, "", "")
	addTestClientServer(c, jujuMetaInstanceServer, "alien")
	addTestClientServer(c, jujuMetaInstanceController, "alien")
	addTestClientServer(c, jujuMetaInstanceServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
	addTestClientServer(c, jujuMetaInstanceServer, "f54aac3a-9dcd-4a0c-86b5-24091478478c")
	suuid := addTestClientServer(c, jujuMetaInstanceController, "f54aac3a-9dcd-4a0c-86b5-24091478478c")

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

	ids, err := cli.getControllerIds()
	c.Check(err, gc.IsNil)
	c.Check(len(ids), gc.Equals, 1)
	c.Check(string(ids[0]), gc.Equals, suuid)
}

func (s *clientSuite) TestClientStopStateInstance(c *gc.C) {
	addTestClientServer(c, "", "")

	addTestClientServer(c, jujuMetaInstanceServer, "alien")
	addTestClientServer(c, jujuMetaInstanceController, "alien")
	addTestClientServer(c, jujuMetaInstanceServer, "client-test")
	suuid := addTestClientServer(c, jujuMetaInstanceController, "client-test")

	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	err = cli.stopInstance(instance.Id(suuid))
	c.Assert(err, gc.IsNil)

	_, err = cli.getControllerIds()
	c.Check(err, gc.Equals, environs.ErrNotBootstrapped)
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

	_, err = cli.getControllerIds()
	c.Check(err, gc.ErrorMatches, "broken connection")
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
	server, drive, arch, err := cli.newInstance(params, img, nil, "")
	c.Check(server, gc.IsNil)
	c.Check(arch, gc.Equals, "")
	c.Check(drive, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "invalid configuration for new instance: InstanceConfig is nil")
}

func (s *clientSuite) TestClientNewInstanceInvalidTemplate(c *gc.C) {
	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	params := environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{},
	}
	err = params.InstanceConfig.SetTools(tools.List{
		&tools.Tools{
			Version: version.Binary{
				Series: "trusty",
			},
			URL: "https://0.1.2.3:2000/x.y.z.tgz",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	img := &imagemetadata.ImageMetadata{
		Id: "invalid-id",
	}
	server, drive, arch, err := cli.newInstance(params, img, nil, "")
	c.Check(server, gc.IsNil)
	c.Check(arch, gc.Equals, "")
	c.Check(drive, gc.IsNil)
	c.Check(err, gc.ErrorMatches, "Failed to query drive template: 404 Not Found, notexist, notfound")
}

func (s *clientSuite) TestClientNewInstance(c *gc.C) {
	cli, err := testNewClient(c, mock.Endpoint(""), mock.TestUser, mock.TestPassword)
	c.Assert(err, gc.IsNil)

	cli.conn.OperationTimeout(1 * time.Second)

	params := environs.StartInstanceParams{
		InstanceConfig: &instancecfg.InstanceConfig{},
	}
	err = params.InstanceConfig.SetTools(tools.List{
		&tools.Tools{
			Version: version.Binary{
				Series: "trusty",
			},
			URL: "https://0.1.2.3:2000/x.y.z.tgz",
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	img := &imagemetadata.ImageMetadata{
		Id: validImageId,
	}
	cs := newConstraints(true, params.Constraints, img)
	c.Assert(cs, gc.NotNil)
	c.Check(err, gc.IsNil)

	templateDrive := &data.Drive{
		Resource: data.Resource{URI: "uri", UUID: cs.driveTemplate},
		LibraryDrive: data.LibraryDrive{
			Arch:      "64",
			ImageType: "image-type",
			OS:        "os",
			Paid:      true,
		},
		Size:   2200 * gosigma.Megabyte,
		Status: "unmounted",
	}
	mock.ResetDrives()
	mock.LibDrives.Add(templateDrive)

	server, drive, arch, err := cli.newInstance(params, img, utils.Gzip([]byte{}), "")
	c.Check(server, gc.NotNil)
	c.Check(drive, gc.NotNil)
	c.Check(arch, gc.NotNil)
	c.Check(err, gc.IsNil)
}
