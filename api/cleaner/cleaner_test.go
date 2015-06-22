// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cleaner_test

import (
	"errors"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/cleaner"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type CleanerSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&CleanerSuite{})

type TestCommon struct {
	apiCaller base.APICaller
	apiArgs   apitesting.CheckArgs
	called    chan struct{}
	api       *cleaner.API
}

// Init returns a new, initialised instance of TestCommon.
func Init(c *gc.C, method string, expectArgs, useResults interface{}, err error) (t *TestCommon) {
	t = &TestCommon{}
	t.apiArgs = apitesting.CheckArgs{
		Facade:        "Cleaner",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	}

	t.called = make(chan struct{}, 100)
	t.apiCaller = apitesting.NotifyingCheckingAPICaller(c, &t.apiArgs, t.called, err)
	t.api = cleaner.NewAPI(t.apiCaller)

	c.Check(t.api, gc.NotNil)
	return
}

// AssertNumReceives checks that the watched channel receives "expected" messages
// within a LongWait, but returns as soon as possible.
func AssertNumReceives(c *gc.C, watched chan struct{}, expected uint32) {
	var receives uint32

	for receives < expected {
		select {
		case <-watched:
			receives++
		case <-time.After(coretesting.LongWait):
			c.Errorf("timeout while waiting for a call")
		}
	}

	time.Sleep(coretesting.ShortWait)
	c.Assert(receives, gc.Equals, expected)
}

func (s *CleanerSuite) TestNewAPI(c *gc.C) {
	Init(c, "", nil, nil, nil)
}

func (s *CleanerSuite) TestWatchCleanups(c *gc.C) {
	t := Init(c, "", nil, nil, nil)
	t.apiArgs.Facade = "" // Multiple facades are called, so we can't check this.
	m, err := t.api.WatchCleanups()
	AssertNumReceives(c, t.called, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.NotNil)
}

func (s *CleanerSuite) TestCleanup(c *gc.C) {
	t := Init(c, "Cleanup", nil, nil, nil)
	err := t.api.Cleanup()
	AssertNumReceives(c, t.called, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CleanerSuite) TestWatchCleanupsFailFacadeCall(c *gc.C) {
	t := Init(c, "WatchCleanups", nil, nil, errors.New("client error!"))
	m, err := t.api.WatchCleanups()
	c.Assert(err, gc.ErrorMatches, "client error!")
	AssertNumReceives(c, t.called, 1)
	c.Assert(m, gc.IsNil)
}

func (s *CleanerSuite) TestWatchCleanupsFailFacadeResult(c *gc.C) {
	e := params.Error{
		Message: "Server Error",
	}
	p := params.NotifyWatchResult{
		Error: &e,
	}
	t := Init(c, "WatchCleanups", nil, p, nil)
	m, err := t.api.WatchCleanups()
	AssertNumReceives(c, t.called, 1)
	c.Assert(err, gc.ErrorMatches, e.Message)
	c.Assert(m, gc.IsNil)
}
