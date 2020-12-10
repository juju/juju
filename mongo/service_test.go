// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	gc "gopkg.in/check.v1"

	coretesting "github.com/juju/juju/testing"
)

type serviceSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestNewConfSnap(c *gc.C) {
	//dataDir := "/var/lib/juju"
	//dbDir := dataDir + "/db"
	//mongodPath := "/usr/bin/mongod"
	//port := 12345
	//oplogSizeMB := 10
	//confArgs := mongo.ConfigArgs{
	//	DataDir:               dataDir,
	//	DBDir:                 dbDir,
	//	MongoPath:             mongodPath,
	//	Port:                  port,
	//	OplogSizeMB:           oplogSizeMB,
	//	WantNUMACtl:           false,
	//	IPv6:                  true,
	//	ReplicaSet:            "juju",
	//	MemoryProfile:         mongo.MemoryProfileLow,
	//	PEMKeyFile:            "/var/lib/juju/server.pem",
	//	PEMKeyPassword:        "ignored",
	//	AuthKeyFile:           "/var/lib/juju/shared-secret",
	//	Syslog:                true,
	//	Journal:               true,
	//	Quiet:                 true,
	//	SSLMode:               "requireSSL",
	//	WiredTigerCacheSizeGB: 0.25,
	//	BindToAllIP:           true,
	//}
	//conf := mongo.NewConf(&confArgs)
	//
	//expected := common.Conf{
	//	Desc:      "juju state database",
	//	Limit:     expectedLimits,
	//	Timeout:   300,
	//	ExecStart: "snap start --enable juju-db",
	//}
	//c.Check(strings.Fields(conf.ExecStart), jc.SameContents, strings.Fields(expected.ExecStart))
}

var expectedLimits = map[string]string{
	"fsize":   "unlimited", // file size
	"cpu":     "unlimited", // cpu time
	"as":      "unlimited", // virtual memory size
	"memlock": "unlimited", // locked-in-memory size
	"nofile":  "64000",     // open files
	"nproc":   "64000",     // processes/threads
}
