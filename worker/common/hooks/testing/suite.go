// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"fmt"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/charm.v6"

	jujutesting "github.com/juju/juju/testing"
	"github.com/juju/juju/worker/common/hooks"
)

func cmdString(cmd string) string {
	return cmd + hooks.CmdSuffix
}

func namedCommandsFunc(name string, f hooks.NewCommandFunc) hooks.EnabledCommandsFunc {
	return func() map[string]hooks.NewCommandFunc {
		return map[string]hooks.NewCommandFunc{
			name: f,
		}
	}
}

func NewCommand(ctx *FakeHookContext, name string, f hooks.NewCommandFunc) (cmd.Command, error) {
	name = cmdString(name)
	return hooks.NewCommand(ctx, name, namedCommandsFunc(name, f))
}

type ContextSuite struct {
	jujutesting.BaseSuite

	Stub *testing.Stub
	Unit string
	rels map[int]*ContextRelation
}

func (s *ContextSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.Stub = &testing.Stub{}
	s.Unit = "u/0"
}

// NewInfo builds a ContextInfo with basic default data.
func (s *ContextSuite) NewInfo() *ContextInfo {
	var info ContextInfo
	info.Unit.Name = s.Unit
	info.ConfigSettings = charm.Settings{
		"empty":               nil,
		"monsters":            false,
		"spline-reticulation": 45.0,
		"title":               "My Title",
		"username":            "admin001",
	}
	info.AvailabilityZone = "us-east-1a"
	info.PublicAddress = "gimli.minecraft.testing.invalid"
	info.PrivateAddress = "192.168.0.99"
	return &info
}

// NewHookContext builds a hooks.Context test double.
func (s *ContextSuite) NewHookContextAndInfo() (*Context, *ContextInfo) {
	info := s.NewInfo()
	return NewContext(s.Stub, info), info
}

func (s *ContextSuite) NewHookContext(c *gc.C) *FakeHookContext {
	hctx, info := s.NewHookContextAndInfo()
	return &FakeHookContext{
		Context: hctx,
		Info:    info,
	}
}

func (s *ContextSuite) GetHookContext(c *gc.C, relid int, remote string) *FakeHookContext {
	c.Assert(relid, gc.Equals, -1)
	return s.NewHookContext(c)
}

func (s *ContextSuite) GetStatusHookContext(c *gc.C) *FakeHookContext {
	return s.NewHookContext(c)
}

type FakeHookContext struct {
	hooks.Context

	Info          *ContextInfo
	Metrics       []hooks.Metric
	CanAddMetrics bool

	RebootPriority hooks.RebootPriority
	ShouldError    bool
}

func (c *FakeHookContext) AddMetric(key, value string, created time.Time) error {
	if !c.CanAddMetrics {
		return fmt.Errorf("metrics disabled")
	}
	c.Metrics = append(c.Metrics, hooks.Metric{
		Key:   key,
		Value: value,
		Time:  created,
	})
	return c.Context.AddMetric(key, value, created)
}

func (c *FakeHookContext) RequestReboot(priority hooks.RebootPriority) error {
	c.RebootPriority = priority
	if c.ShouldError {
		return fmt.Errorf("RequestReboot error!")
	} else {
		return nil
	}
}
