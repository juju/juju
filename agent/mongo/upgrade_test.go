// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"labix.org/v2/mgo"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/agent/mongo"
	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/upstart"
)

type EnsureAdminSuite struct {
	coretesting.BaseSuite
	serviceStarts int
	serviceStops  int
}

var _ = gc.Suite(&EnsureAdminSuite{})

func (s *EnsureAdminSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.serviceStarts = 0
	s.serviceStops = 0
	s.PatchValue(mongo.UpstartConfInstall, func(conf *upstart.Conf) error {
		return nil
	})
	s.PatchValue(mongo.UpstartServiceStart, func(svc *upstart.Service) error {
		s.serviceStarts++
		return nil
	})
	s.PatchValue(mongo.UpstartServiceStop, func(svc *upstart.Service) error {
		s.serviceStops++
		return nil
	})
}

func (s *EnsureAdminSuite) TestEnsureAdminUser(c *gc.C) {
	inst := &coretesting.MgoInstance{}
	err := inst.Start(true)
	c.Assert(err, gc.IsNil)
	defer inst.DestroyWithLog()
	dialInfo := inst.DialInfo()

	// Mock out mongod, so the --noauth execution doesn't
	// do anything nasty. Also mock out the Signal method.
	jujutesting.PatchExecutableAsEchoArgs(c, s, "mongod")
	mongodDir := filepath.SplitList(os.Getenv("PATH"))[0]
	s.PatchValue(&mongo.JujuMongodPath, filepath.Join(mongodDir, "mongod"))
	s.PatchValue(mongo.ProcessSignal, func(*os.Process, os.Signal) error {
		return nil
	})

	// First call succeeds, as there are no users yet.
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsTrue)

	// EnsureAdminUser should have stopped the mongo service,
	// started a new mongod with --noauth, and then finally
	// started the service back up.
	c.Assert(s.serviceStarts, gc.Equals, 1)
	c.Assert(s.serviceStops, gc.Equals, 1)
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, gc.IsNil)
	jujutesting.AssertEchoArgs(c, "mongod",
		"--noauth",
		"--dbpath", "db",
		"--sslOnNormalPorts",
		"--sslPEMKeyFile", "server.pem",
		"--sslPEMKeyPassword", "ignored",
		"--bind_ip", "127.0.0.1",
		"--port", portString,
		"--noprealloc",
		"--syslog",
		"--smallfiles",
		"--journal",
	)

	// Second call succeeds, as the admin user is already there.
	added, err = s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsFalse)

	// There should have been no additional start/stop.
	c.Assert(s.serviceStarts, gc.Equals, 1)
	c.Assert(s.serviceStops, gc.Equals, 1)
}

func (s *EnsureAdminSuite) TestEnsureAdminUserError(c *gc.C) {
	inst := &coretesting.MgoInstance{}
	err := inst.Start(true)
	c.Assert(err, gc.IsNil)
	defer inst.Destroy()
	dialInfo := inst.DialInfo()

	// First call succeeds, as there are no users yet (mimics --noauth).
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsTrue)

	// Second call fails, as there is another user and the database doesn't
	// actually get reopened with --noauth in the test; mimics AddUser failure
	_, err = s.ensureAdminUser(c, dialInfo, "whomeverelse", "whateverelse")
	c.Assert(err, gc.ErrorMatches, `failed to add "whomeverelse" to admin database: not authorized for upsert on admin.system.users`)
}

func (s *EnsureAdminSuite) ensureAdminUser(c *gc.C, dialInfo *mgo.DialInfo, user, password string) (added bool, err error) {
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, gc.IsNil)
	port, err := strconv.Atoi(portString)
	c.Assert(err, gc.IsNil)
	return mongo.EnsureAdminUser(mongo.EnsureAdminUserParams{
		DialInfo: dialInfo,
		Port:     port,
		User:     user,
		Password: password,
	})
}
