// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type rateLimitSuite struct {
	jujutesting.ApiServerSuite
}

var _ = gc.Suite(&rateLimitSuite{})

func (s *rateLimitSuite) SetUpTest(c *gc.C) {
	s.Clock = testclock.NewDilatedWallClock(time.Second)
	s.ControllerConfigAttrs = map[string]interface{}{
		corecontroller.AgentRateLimitMax:  1,
		corecontroller.AgentRateLimitRate: (60 * time.Second).String(),
	}
	s.ApiServerSuite.SetUpTest(c)
}

func (s *rateLimitSuite) TestRateLimitAgents(c *gc.C) {
	c.Assert(s.Server.Report(), jc.DeepEquals, map[string]interface{}{
		"agent-ratelimit-max":  1,
		"agent-ratelimit-rate": 60 * time.Second,
	})

	info := s.ControllerModelApiInfo()
	// First agent connection is fine.
	machine1 := s.infoForNewMachine(c, info)
	conn1, err := api.Open(machine1, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn1.Close()

	// Second machine in the same minute gets told to go away and try again.
	machine2 := s.infoForNewMachine(c, info)
	_, err = api.Open(machine2, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, `try again \(try again\)`)

	// If we wait a minute and try again, it is fine.
	s.Clock.Advance(time.Minute)
	conn2, err := api.Open(machine2, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn2.Close()

	// And the next one is limited.
	machine3 := s.infoForNewMachine(c, info)
	_, err = api.Open(machine3, fastDialOpts)
	c.Assert(err, gc.ErrorMatches, `try again \(try again\)`)
}

func (s *rateLimitSuite) TestRateLimitNotApplicableToUsers(c *gc.C) {
	info := s.ControllerModelApiInfo()

	// First agent connection is fine.
	machine1 := s.infoForNewMachine(c, info)
	conn1, err := api.Open(machine1, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn1.Close()

	// User connections are fine.
	user := s.infoForNewUser(c, info, "fredrikthordendal")
	conn2, err := api.Open(user, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn2.Close()

	user2 := s.infoForNewUser(c, info, "jenskidman")
	conn3, err := api.Open(user2, fastDialOpts)
	c.Assert(err, jc.ErrorIsNil)
	defer conn3.Close()
}

func (s *rateLimitSuite) infoForNewMachine(c *gc.C, info *api.Info) *api.Info {
	// Make a copy
	newInfo := *info

	f, release := s.NewFactory(c, info.ModelTag.Id())
	defer release()
	machine, password := f.MakeMachineReturningPassword(
		c, &factory.MachineParams{Nonce: "fake_nonce"})

	newInfo.Tag = machine.Tag()
	newInfo.Password = password
	newInfo.Nonce = "fake_nonce"
	return &newInfo
}

func (s *rateLimitSuite) infoForNewUser(c *gc.C, info *api.Info, name string) *api.Info {
	// Make a copy
	newInfo := *info

	accessService := s.ControllerServiceFactory(c).Access()

	userTag := names.NewUserTag(name)
	_, _, err := accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        userTag.Name(),
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
		Permission:  permission.ControllerForAccess(permission.LoginAccess),
	})
	c.Assert(err, jc.ErrorIsNil)

	_, err = accessService.CreatePermission(context.Background(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.AdminAccess,
		},
		User: userTag.Name(),
	})
	c.Assert(err, jc.ErrorIsNil)

	newInfo.Tag = userTag
	newInfo.Password = "hunter2"
	return &newInfo
}
