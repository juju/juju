// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
)

type serviceSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestNewConf(c *gc.C) {
	dataDir := "/var/lib/juju"
	dbDir := dataDir + "/db"
	mongodPath := "/mgo/bin/mongod"
	mongodVersion := mongo.Mongo24
	port := 12345
	oplogSizeMB := 10
	conf := mongo.NewConf(mongo.ConfigArgs{
		DataDir:     dataDir,
		DBDir:       dbDir,
		MongoPath:   mongodPath,
		Port:        port,
		OplogSizeMB: oplogSizeMB,
		WantNumaCtl: false,
		Version:     mongodVersion,
		Auth:        true,
		IPv6:        true,
	})

	expected := common.Conf{
		Desc: "juju state database",
		Limit: map[string]int{
			"nofile": 65000,
			"nproc":  20000,
		},
		Timeout: 300,
		ExecStart: "/mgo/bin/mongod" +
			" --dbpath '/var/lib/juju/db'" +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile '/var/lib/juju/server.pem'" +
			" --sslPEMKeyPassword=ignored" +
			" --port 12345" +
			" --syslog" +
			" --journal" +
			" --replSet juju" +
			" --quiet" +
			" --oplogSize 10" +
			" --ipv6" +
			" --auth" +
			" --keyFile '/var/lib/juju/shared-secret'" +
			" --noprealloc" +
			" --smallfiles",
	}

	c.Check(conf, jc.DeepEquals, expected)
	c.Check(strings.Fields(conf.ExecStart), jc.DeepEquals, strings.Fields(expected.ExecStart))
}

func (s *serviceSuite) TestIsServiceInstalledWhenInstalled(c *gc.C) {
	svcName := mongo.ServiceName
	svcData := svctesting.NewFakeServiceData(svcName)
	mongo.PatchService(s.PatchValue, svcData)

	isInstalled, err := mongo.IsServiceInstalled()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isInstalled, jc.IsTrue)
}

func (s *serviceSuite) TestIsServiceInstalledWhenNotInstalled(c *gc.C) {
	mongo.PatchService(s.PatchValue, svctesting.NewFakeServiceData())

	isInstalled, err := mongo.IsServiceInstalled()

	c.Assert(err, jc.ErrorIsNil)
	c.Assert(isInstalled, jc.IsFalse)
}
