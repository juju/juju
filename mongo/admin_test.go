// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo_test

import (
	"github.com/juju/mgo/v3"
	"github.com/juju/mgo/v3/bson"
	mgotesting "github.com/juju/mgo/v3/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging"
	"github.com/juju/juju/mongo"
	coretesting "github.com/juju/juju/testing"
)

type adminSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&adminSuite{})

func (s *adminSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.PatchValue(mongo.InstallMongo, func(dep packaging.Dependency, series string) error {
		return nil
	})
}

func (s *adminSuite) setUpMongo(c *gc.C) *mgo.DialInfo {
	inst := &mgotesting.MgoInstance{
		EnableReplicaSet: true,
	}
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
			if role["db"] == db {
				roles = append(roles, role["role"])
			}
		default:
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
	c.Assert(err, gc.ErrorMatches, "(auth|(.*Authentication)) fail(s|ed)\\.?")
	err = admin.Login("auser", "foo")
	c.Assert(err, jc.ErrorIsNil)

	checkRoles(c, session, "admin", "auser",
		[]interface{}{
			string(mgo.RoleReadWriteAny),
			string(mgo.RoleDBAdminAny),
			string(mgo.RoleUserAdminAny),
			string(mgo.RoleClusterAdmin)})
}
