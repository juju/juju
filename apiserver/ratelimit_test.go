// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	corecontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	"github.com/juju/juju/internal/testing/factory"
	jujutesting "github.com/juju/juju/juju/testing"
)

type rateLimitSuite struct {
	jujutesting.ApiServerSuite
}

var _ = tc.Suite(&rateLimitSuite{})

func (s *rateLimitSuite) SetUpTest(c *tc.C) {
	s.Clock = testclock.NewDilatedWallClock(time.Second)
	s.ControllerConfigAttrs = map[string]interface{}{
		corecontroller.AgentRateLimitMax:  1,
		corecontroller.AgentRateLimitRate: (60 * time.Second).String(),
	}
	s.ApiServerSuite.SetUpTest(c)
}

func (s *rateLimitSuite) TestRateLimitAgents(c *tc.C) {
	c.Assert(s.Server.Report(), tc.DeepEquals, map[string]interface{}{
		"agent-ratelimit-max":  1,
		"agent-ratelimit-rate": 60 * time.Second,
	})

	info := s.ControllerModelApiInfo()
	// First agent connection is fine.
	machine1 := s.infoForNewMachine(c, info)
	conn1, err := api.Open(context.Background(), machine1, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer conn1.Close()

	// Second machine in the same minute gets told to go away and try again.
	machine2 := s.infoForNewMachine(c, info)
	_, err = api.Open(context.Background(), machine2, fastDialOpts)
	c.Assert(err, tc.ErrorMatches, `try again \(try again\)`)

	// If we wait a minute and try again, it is fine.
	s.Clock.Advance(time.Minute)
	conn2, err := api.Open(context.Background(), machine2, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer conn2.Close()

	// And the next one is limited.
	machine3 := s.infoForNewMachine(c, info)
	_, err = api.Open(context.Background(), machine3, fastDialOpts)
	c.Assert(err, tc.ErrorMatches, `try again \(try again\)`)
}

func (s *rateLimitSuite) TestRateLimitNotApplicableToUsers(c *tc.C) {
	info := s.ControllerModelApiInfo()

	// First agent connection is fine.
	machine1 := s.infoForNewMachine(c, info)
	conn1, err := api.Open(context.Background(), machine1, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer conn1.Close()

	// User connections are fine.
	user := s.infoForNewUser(c, info, "fredrikthordendal")
	conn2, err := api.Open(context.Background(), user, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer conn2.Close()

	user2 := s.infoForNewUser(c, info, "jenskidman")
	conn3, err := api.Open(context.Background(), user2, fastDialOpts)
	c.Assert(err, tc.ErrorIsNil)
	defer conn3.Close()
}

func (s *rateLimitSuite) infoForNewMachine(c *tc.C, info *api.Info) *api.Info {
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

func (s *rateLimitSuite) infoForNewUser(c *tc.C, info *api.Info, name string) *api.Info {
	// Make a copy
	newInfo := *info

	accessService := s.ControllerDomainServices(c).Access()

	userTag := names.NewUserTag(name)
	_, _, err := accessService.AddUser(context.Background(), service.AddUserArg{
		Name:        user.NameFromTag(userTag),
		CreatorUUID: s.AdminUserUUID,
		Password:    ptr(auth.NewPassword("hunter2")),
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	_, err = accessService.CreatePermission(context.Background(), permission.UserAccessSpec{
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Model,
				Key:        s.ControllerModelUUID(),
			},
			Access: permission.AdminAccess,
		},
		User: user.NameFromTag(userTag),
	})
	c.Assert(err, tc.ErrorIsNil)

	newInfo.Tag = userTag
	newInfo.Password = "hunter2"
	return &newInfo
}
