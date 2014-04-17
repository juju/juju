// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package mongo

import (
	"net"
	"strconv"

	jc "github.com/juju/testing/checkers"
	"labix.org/v2/mgo"
	gc "launchpad.net/gocheck"

	coretesting "launchpad.net/juju-core/testing"
	"launchpad.net/juju-core/testing/testbase"
	"launchpad.net/juju-core/upstart"
)

type EnsureAdminSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&EnsureAdminSuite{})

func (s *EnsureAdminSuite) SetUpTest(c *gc.C) {
	s.PatchValue(&upstartConfInstall, func(conf *upstart.Conf) error {
		return nil
	})
	s.PatchValue(&upstartServiceStart, func(svc *upstart.Service) error {
		return nil
	})
	s.PatchValue(&upstartServiceStop, func(svc *upstart.Service) error {
		return nil
	})
}

func (s *EnsureAdminSuite) TestEnsureAdminUser(c *gc.C) {
	inst := &coretesting.MgoInstance{}
	err := inst.Start(true)
	c.Assert(err, gc.IsNil)
	defer inst.Destroy()
	dialInfo := inst.DialInfo()
	// First call succeeds, as there are no users yet.
	added, err := s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsTrue)
	// Second call succeeds, as the admin user is already there.
	added, err = s.ensureAdminUser(c, dialInfo, "whomever", "whatever")
	c.Assert(err, gc.IsNil)
	c.Assert(added, jc.IsFalse)
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
	return EnsureAdminUser(EnsureAdminUserParams{
		DialInfo: dialInfo,
		Port:     port,
		User:     user,
		Password: password,
	})
}
