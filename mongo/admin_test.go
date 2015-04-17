// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"net"
	"os"
	"path/filepath"
	"strconv"

	gitjujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/juju/juju/mongo"
	svctesting "github.com/juju/juju/service/common/testing"
	coretesting "github.com/juju/juju/testing"
)

type adminSuite struct {
	coretesting.BaseSuite

	data *svctesting.FakeServiceData
}

var _ = gc.Suite(&adminSuite{})

func (s *adminSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.data = svctesting.NewFakeServiceData()
	mongo.PatchService(s.PatchValue, s.data)
}

func (s *adminSuite) TestEnsureAdminUser(c *gc.C) {
	inst := &gitjujutesting.MgoInstance{}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.DestroyWithLog()
	dialInfo := inst.DialInfo()

	// Mock out mongod, so the --noauth execution doesn't
	// do anything nasty. Also mock out the Signal method.
	gitjujutesting.PatchExecutableAsEchoArgs(c, s, "mongod")
	mongodDir := filepath.SplitList(os.Getenv("PATH"))[0]
	s.PatchValue(&mongo.JujuMongodPath, filepath.Join(mongodDir, "mongod"))
	s.PatchValue(mongo.ProcessSignal, func(*os.Process, os.Signal) error {
		return nil
	})

	// First call succeeds, as there are no users yet.
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(added, jc.IsTrue)

	// EnsureAdminUser should have stopped the mongo service,
	// started a new mongod with --noauth, and then finally
	// started the service back up.
	s.data.CheckCallNames(c, "Stop", "Start")
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)
	gitjujutesting.AssertEchoArgs(c, "mongod",
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
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(added, jc.IsFalse)

	// There should have been no additional start/stop.
	s.data.CheckCallNames(c, "Stop", "Start")
}

func (s *adminSuite) TestEnsureAdminUserError(c *gc.C) {
	inst := &gitjujutesting.MgoInstance{}
	inst.EnableAuth = true
	err := inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	defer inst.Destroy()
	dialInfo := inst.DialInfo()

	// First call succeeds, as there are no users yet (mimics --noauth).
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(added, jc.IsTrue)

	// Second call fails, as there is another user and the database doesn't
	// actually get reopened with --noauth in the test; mimics AddUser failure
	_, err = s.ensureAdminUser(c, dialInfo, "whomeverelse", "whateverelse")
	c.Assert(err, gc.ErrorMatches, `failed to add "whomeverelse" to admin database: cannot set admin password: not authorized .*`)
}

func (s *adminSuite) ensureAdminUser(c *gc.C, dialInfo *mgo.DialInfo, user, password string) (added bool, err error) {
	_, portString, err := net.SplitHostPort(dialInfo.Addrs[0])
	c.Assert(err, jc.ErrorIsNil)
	port, err := strconv.Atoi(portString)
	c.Assert(err, jc.ErrorIsNil)
	return mongo.EnsureAdminUser(mongo.EnsureAdminUserParams{
		DialInfo: dialInfo,
		Port:     port,
		User:     user,
		Password: password,
	})
}

func (s *adminSuite) setUpMongo(c *gc.C) *mgo.DialInfo {
	inst := &gitjujutesting.MgoInstance{}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, jc.ErrorIsNil)
	s.AddCleanup(func(*gc.C) { inst.Destroy() })
	dialInfo := inst.DialInfo()
	dialInfo.Direct = true
	return dialInfo
}

func checkRoles(c *gc.C, session *mgo.Session, db, user string, expected []interface{}) {
	admin := session.DB("admin")

	var info map[string]interface{}
	err := admin.C("system.users").Find(bson.D{{"user", user}}).One(&info)
	c.Assert(err, jc.ErrorIsNil)

	var roles []interface{}
	for _, role := range info["roles"].([]interface{}) {
		switch role := role.(type) {
		case map[string]interface{}:
			// Mongo 2.6
			if role["db"] == db {
				roles = append(roles, role["role"])
			}
		default:
			// Mongo 2.4
			roles = append(roles, role)
		}
	}
	c.Assert(roles, jc.SameContents, expected)
}

func (s *adminSuite) TestSetAdminMongoPassword(c *gc.C) {
	dialInfo := s.setUpMongo(c)
	session, err := mgo.DialWithInfo(dialInfo)
	c.Assert(err, jc.ErrorIsNil)
	defer session.Close()

	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err = mongo.SetAdminMongoPassword(session, "auser", "")
	c.Assert(err, jc.ErrorIsNil)

	admin := session.DB("admin")
	err = mongo.SetAdminMongoPassword(session, "auser", "foo")
	c.Assert(err, jc.ErrorIsNil)
	err = admin.Login("auser", "")
	c.Assert(err, gc.ErrorMatches, "auth fail(s|ed)")
	err = admin.Login("auser", "foo")
	c.Assert(err, jc.ErrorIsNil)

	checkRoles(c, session, "admin", "auser",
		[]interface{}{
			string(mgo.RoleReadWriteAny),
			string(mgo.RoleDBAdminAny),
			string(mgo.RoleUserAdminAny),
			string(mgo.RoleClusterAdmin)})
}
