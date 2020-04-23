// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"strings"

	"github.com/juju/collections/set"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/mongo"
	"github.com/juju/juju/service/common"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
)

var logger = loggo.GetLogger("juju.mongo.service_test")

type serviceSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&serviceSuite{})

func (s *serviceSuite) TestNewConf24(c *gc.C) {
	dataDir := "/var/lib/juju"
	mongodPath := "/mgo/bin/mongod"
	mongodVersion := mongo.Mongo24
	port := 12345
	oplogSizeMB := 10
	usingMongoFromSnap := false

	confArgs := mongo.GenerateConf(
		mongodPath,
		oplogSizeMB,
		mongodVersion,
		usingMongoFromSnap,
		mongo.EnsureServerParams{
			DataDir:       dataDir,
			StatePort:     port,
			MemoryProfile: mongo.MemoryProfileDefault,
		},
	)
	conf := mongo.NewConf(confArgs)

	expectedArgs := set.NewStrings(
		"--dbpath",
		"--sslPEMKeyPassword=ignored",
		"--port",
		"--syslog",
		"--journal",
		"--port",
		"--quiet",
		"--oplogSize",
		"--ipv6",
		"--auth",
		"--sslOnNormalPorts",
		"--keyFile",
		"--sslPEMKeyFile",
		"--replSet",
		// required when MongoDB 2.4 is deployed
		"--noprealloc",
		"--smallfiles",
	)

	expectedKwArgs := map[string]string{
		"--dbpath":        "'/var/lib/juju/db'",
		"--sslPEMKeyFile": "'/var/lib/juju/server.pem'",
		"--port":          "12345",
		" --keyFile":      "'/var/lib/juju/shared-secret'",
		" --oplogSize":    "10",
		" --replSet":      "juju",
	}

	c.Assert(conf.Desc, gc.Not(gc.Equals), "")
	confFields := strings.Fields(conf.ExecStart)
	for i, field := range confFields {
		if !strings.HasPrefix(field, "-") {
			continue
		}
		logger.Debugf("checking argument %v", field)
		c.Assert(expectedArgs.Contains(field), gc.Equals, true)
		expectedArgs.Remove(field)

		expectedVal, ok := expectedKwArgs[field]
		if ok {
			actualVal := confFields[i+1]
			c.Assert(expectedVal, gc.Equals, actualVal)
		}
	}
	logger.Debugf("expectedArgs contents %v", expectedArgs)
	c.Assert(expectedArgs.IsEmpty(), gc.Equals, true)
}

func (s *serviceSuite) TestNewConf32(c *gc.C) {
	dataDir := "/var/lib/juju"
	mongodPath := "/mgo/bin/mongod"
	mongodVersion := mongo.Mongo32wt
	port := 12345
	oplogSizeMB := 10
	usingMongoFromSnap := false
	confArgs := mongo.GenerateConf(
		mongodPath,
		oplogSizeMB,
		mongodVersion,
		usingMongoFromSnap,
		mongo.EnsureServerParams{
			DataDir:       dataDir,
			StatePort:     port,
			MemoryProfile: mongo.MemoryProfileDefault,
		},
	)
	conf := mongo.NewConf(confArgs)

	confFields := strings.Fields(conf.ExecStart)
	hasStorageEngine := false
	hasOplogSize := false
	hasCacheSize := false

	for i, field := range confFields {
		if field == "--storageEngine" {
			hasStorageEngine = true
			c.Check(confFields[i+1], gc.Equals, "wiredTiger")
		} else if field == "--oplogSize" {
			hasOplogSize = true
			c.Check(confFields[i+1], gc.Equals, "10")
		} else if field == "--wiredTigerCacheSizeGB" {
			hasCacheSize = true
		}
	}
	c.Check(hasStorageEngine, gc.Equals, true)
	c.Check(hasOplogSize, gc.Equals, true)
	c.Check(hasCacheSize, gc.Equals, false)
}

func (s *serviceSuite) TestNewConf32LowMem(c *gc.C) {
	dataDir := "/var/lib/juju"
	mongodPath := "/mgo/bin/mongod"
	mongodVersion := mongo.Mongo32wt
	port := 12345
	oplogSizeMB := 10
	usingMongoFromSnap := false

	confArgs := mongo.GenerateConf(
		mongodPath,
		oplogSizeMB,
		mongodVersion,
		usingMongoFromSnap,
		mongo.EnsureServerParams{
			DataDir:       dataDir,
			StatePort:     port,
			MemoryProfile: mongo.MemoryProfileLow,
		},
	)
	conf := mongo.NewConf(confArgs)

	expectedArgs := set.NewStrings(
		"--dbpath",
		"--sslPEMKeyPassword=ignored",
		"--port",
		"--syslog",
		"--journal",
		"--port",
		"--quiet",
		"--oplogSize",
		"--ipv6",
		"--auth",
		"--keyFile",
		"--sslPEMKeyFile",
		"--replSet",
		// required when MongoDB 3.2 is deployed
		"--storageEngine",
		"--wiredTigerCacheSizeGB",
		"--sslMode",
	)

	expectedKwArgs := map[string]string{
		"--dbpath":                "'/var/lib/juju/db'",
		"--sslPEMKeyFile":         "'/var/lib/juju/server.pem'",
		"--port":                  "12345",
		"--keyFile":               "'/var/lib/juju/shared-secret'",
		"--oplogSize":             "10",
		"--replSet":               "juju",
		"--storageEngine":         "wiredTiger",
		"--wiredTigerCacheSizeGB": "1",
		"--sslMode":               "requireSSL",
	}

	c.Assert(conf.Desc, gc.Not(gc.Equals), "")
	confFields := strings.Fields(conf.ExecStart)
	for i, field := range confFields {
		if !strings.HasPrefix(field, "-") {
			continue
		}
		logger.Debugf("checking argument %v", field)
		c.Assert(expectedArgs.Contains(field), gc.Equals, true)
		expectedArgs.Remove(field)

		expectedVal, ok := expectedKwArgs[field]
		if ok {
			actualVal := confFields[i+1]
			c.Assert(expectedVal, gc.Equals, actualVal)
		}
	}
	logger.Debugf("expectedArgs contents %v", expectedArgs.Values())
	c.Assert(expectedArgs.IsEmpty(), gc.Equals, true)
}

func (s *serviceSuite) TestNewConf36(c *gc.C) {
	dataDir := "/var/lib/juju"
	dbDir := dataDir + "/db"
	mongodPath := "/usr/bin/mongod"
	mongodVersion := mongo.Mongo36wt
	port := 12345
	oplogSizeMB := 10
	confArgs := mongo.ConfigArgs{
		DataDir:               dataDir,
		DBDir:                 dbDir,
		MongoPath:             mongodPath,
		Port:                  port,
		OplogSizeMB:           oplogSizeMB,
		WantNUMACtl:           false,
		Version:               mongodVersion,
		IPv6:                  true,
		ReplicaSet:            "juju",
		MemoryProfile:         mongo.MemoryProfileLow,
		PEMKeyFile:            "/var/lib/juju/server.pem",
		PEMKeyPassword:        "ignored",
		AuthKeyFile:           "/var/lib/juju/shared-secret",
		Syslog:                true,
		Journal:               true,
		Quiet:                 true,
		SSLMode:               "requireSSL",
		WiredTigerCacheSizeGB: 0.25,
		BindToAllIP:           true,
	}
	conf := mongo.NewConf(&confArgs)

	expected := common.Conf{
		Desc:    "juju state database",
		Limit:   expectedLimits,
		Timeout: 300,
		ExecStart: "/usr/bin/mongod" +
			" --dbpath '/var/lib/juju/db'" +
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
			" --sslMode requireSSL" +
			" --storageEngine wiredTiger" +
			" --wiredTigerCacheSizeGB 0.25" +
			" --bind_ip_all",
	}
	c.Check(strings.Fields(conf.ExecStart), jc.SameContents, strings.Fields(expected.ExecStart))
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

var expectedLimits = map[string]string{
	"fsize":   "unlimited", // file size
	"cpu":     "unlimited", // cpu time
	"as":      "unlimited", // virtual memory size
	"memlock": "unlimited", // locked-in-memory size
	"nofile":  "64000",     // open files
	"nproc":   "64000",     // processes/threads
}
