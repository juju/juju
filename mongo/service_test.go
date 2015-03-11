// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"strings"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service/common"
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
	port := 12345
	oplogSizeMB := 10
	conf := mongo.NewConf(dataDir, dbDir, mongodPath, port, oplogSizeMB, false)

	expected := common.Conf{
		Desc: "juju state database",
		Limit: map[string]int{
			"nofile": 65000,
			"nproc":  20000,
		},
		Timeout: 300,
		ExecStart: "/mgo/bin/mongod" +
			" --auth" +
			" --dbpath '/var/lib/juju/db'" +
			" --sslOnNormalPorts" +
			" --sslPEMKeyFile '/var/lib/juju/server.pem'" +
			" --sslPEMKeyPassword ignored" +
			" --port 12345" +
			" --noprealloc" +
			" --syslog" +
			" --smallfiles" +
			" --journal" +
			" --keyFile '/var/lib/juju/shared-secret'" +
			" --replSet juju" +
			" --ipv6" +
			" --oplogSize 10",
	}
	c.Check(conf, jc.DeepEquals, expected)
	c.Check(strings.Fields(conf.ExecStart), jc.DeepEquals, strings.Fields(expected.ExecStart))
}
