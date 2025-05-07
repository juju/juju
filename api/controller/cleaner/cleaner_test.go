// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"context"
	"errors"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/controller/cleaner"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
)

type CleanerSuite struct {
	coretesting.BaseSuite
}

var _ = tc.Suite(&CleanerSuite{})

type TestCommon struct {
	apiCaller base.APICaller
	called    chan struct{}
	api       *cleaner.API
}

// Init returns a new, initialised instance of TestCommon.
func Init(c *tc.C, facade, method string, expectArgs, useResults interface{}, err error) (t *TestCommon) {
	t = &TestCommon{}
	caller := apitesting.APICallChecker(c, apitesting.APICall{
		Facade:        facade,
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
		Error:         err,
	})
	t.called = make(chan struct{}, 100)
	t.apiCaller = apitesting.NotifyingAPICaller(c, t.called, caller)
	t.api = cleaner.NewAPI(t.apiCaller)

	c.Check(t.api, tc.NotNil)
	return
}

// AssertNumReceives checks that the watched channel receives "expected" messages
// within a LongWait, but returns as soon as possible.
func AssertNumReceives(c *tc.C, watched chan struct{}, expected uint32) {
	var receives uint32

	for receives < expected {
		select {
		case <-watched:
			receives++
		case <-time.After(coretesting.LongWait):
			c.Errorf("timeout while waiting for a call")
		}
	}
	select {
	case <-watched:
		c.Fatalf("unexpected event received")
	case <-time.After(coretesting.ShortWait):
	}
}

func (s *CleanerSuite) TestNewAPI(c *tc.C) {
	Init(c, "Cleaner", "", nil, nil, nil)
}

func (s *CleanerSuite) TestWatchCleanups(c *tc.C) {
	// Multiple facades are called, so pass an empty string for the facade.
	t := Init(c, "", "", nil, nil, nil)
	m, err := t.api.WatchCleanups(context.Background())
	AssertNumReceives(c, t.called, 2)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(m, tc.NotNil)
}

func (s *CleanerSuite) TestCleanup(c *tc.C) {
	t := Init(c, "Cleaner", "Cleanup", nil, nil, nil)
	err := t.api.Cleanup(context.Background())
	AssertNumReceives(c, t.called, 1)
	c.Assert(err, tc.ErrorIsNil)
}

func (s *CleanerSuite) TestWatchCleanupsFailFacadeCall(c *tc.C) {
	t := Init(c, "Cleaner", "WatchCleanups", nil, nil, errors.New("client error!"))
	m, err := t.api.WatchCleanups(context.Background())
	c.Assert(err, tc.ErrorMatches, "client error!")
	AssertNumReceives(c, t.called, 1)
	c.Assert(m, tc.IsNil)
}

func (s *CleanerSuite) TestWatchCleanupsFailFacadeResult(c *tc.C) {
	e := params.Error{
		Message: "Server Error",
	}
	p := params.NotifyWatchResult{
		Error: &e,
	}
	t := Init(c, "Cleaner", "WatchCleanups", nil, p, nil)
	m, err := t.api.WatchCleanups(context.Background())
	AssertNumReceives(c, t.called, 1)
	c.Assert(err, tc.ErrorMatches, e.Message)
	c.Assert(m, tc.IsNil)
}
