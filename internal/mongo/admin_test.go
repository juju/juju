// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	stdtesting "testing"

	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/mongo"
	coretesting "github.com/juju/juju/internal/testing"
)

type adminSuite struct {
	coretesting.BaseSuite
}

func TestAdminSuite(t *stdtesting.T) {
	tc.Run(t, &adminSuite{})
}

func (s *adminSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(mongo.InstallMongo, func(string) error {
		return nil
	})
}

func (s *adminSuite) setUpMongo(c *tc.C) *mgo.DialInfo {
	inst := &mgotesting.MgoInstance{
		EnableReplicaSet: true,
	}
	err := inst.Start(coretesting.Certs)
	c.Assert(err, tc.ErrorIsNil)
	s.AddCleanup(func(*tc.C) { inst.Destroy() })
	dialInfo := inst.DialInfo()
	dialInfo.Direct = true
	return dialInfo
}

func checkRoles(c *tc.C, session *mgo.Session, db, user string, expected []interface{}) {
	admin := session.DB("admin")

	var info map[string]interface{}
	err := admin.C("system.users").Find(bson.D{{"user", user}}).One(&info)
	c.Assert(err, tc.ErrorIsNil)

	var roles []interface{}
	for _, role := range info["roles"].([]interface{}) {
		switch role := role.(type) {
		case map[string]interface{}:
			if role["db"] == db {
				roles = append(roles, role["role"])
			}
		default:
		}
	}
	c.Assert(roles, tc.SameContents, expected)
}

func (s *adminSuite) TestSetAdminMongoPassword(c *tc.C) {
	dialInfo := s.setUpMongo(c)
	session, err := mgo.DialWithInfo(dialInfo)
	c.Assert(err, tc.ErrorIsNil)
	defer session.Close()

	// Check that we can SetAdminMongoPassword to nothing when there's
	// no password currently set.
	err = mongo.SetAdminMongoPassword(session, "auser", "")
	c.Assert(err, tc.ErrorIsNil)

	admin := session.DB("admin")
	err = mongo.SetAdminMongoPassword(session, "auser", "foo")
	c.Assert(err, tc.ErrorIsNil)
	err = admin.Login("auser", "")
	c.Assert(err, tc.ErrorMatches, "(auth|(.*Authentication)) fail(s|ed)\\.?")
	err = admin.Login("auser", "foo")
	c.Assert(err, tc.ErrorIsNil)

	checkRoles(c, session, "admin", "auser",
		[]interface{}{
			string(mgo.RoleReadWriteAny),
			string(mgo.RoleDBAdminAny),
			string(mgo.RoleUserAdminAny),
			string(mgo.RoleClusterAdmin)})
}
