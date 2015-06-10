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
	numCalls  int
	api       *cleaner.API
}

// Init returns a new, initialised instance of TestCommon.
func Init(c *gc.C, method string, expectArgs, useResults interface{}, err error) (t *TestCommon) {
	t = &TestCommon{}
	t.apiArgs = apitesting.CheckArgs{
		Facade:        "",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
		Results:       useResults,
	}

	t.apiCaller = apitesting.CheckingAPICaller(c, &t.apiArgs, &t.numCalls, err)
	t.api = cleaner.NewAPI(t.apiCaller)

	c.Check(t.api, gc.NotNil)
	c.Check(t.numCalls, gc.Equals, 0)
	return
}

// AssertEventuallyEqual checks that numCalls reaches the value "calls" within a
// LongWait, but returns as soon as possible.
func AssertEventuallyEqual(c *gc.C, watched *int, calls int) {
	ch := make(chan struct{})

	go func() {
		for *watched < calls {
			time.Sleep(coretesting.ShortWait)
		}
		// Wait just in case this could get larger
		time.Sleep(coretesting.ShortWait)
		ch <- struct{}{}
	}()

	select {
	case <-ch:
	case <-time.After(coretesting.LongWait):
	}

	c.Assert(*watched, gc.Equals, calls)
}

func (s *CleanerSuite) TestNewAPI(c *gc.C) {
	t := Init(c, "", nil, nil, nil)
	t.apiArgs.Facade = "Cleaner"
}

func (s *CleanerSuite) TestWatchCleanups(c *gc.C) {
	t := Init(c, "", nil, nil, nil)
	m, err := t.api.WatchCleanups()
	AssertEventuallyEqual(c, &t.numCalls, 2)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(m, gc.NotNil)
}

func (s *CleanerSuite) TestCleanup(c *gc.C) {
	t := Init(c, "", nil, nil, nil)
	err := t.api.Cleanup()
	AssertEventuallyEqual(c, &t.numCalls, 1)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *CleanerSuite) TestWatchCleanupsFailFacadeCall(c *gc.C) {
	t := Init(c, "", nil, nil, errors.New("client error!"))
	m, err := t.api.WatchCleanups()
	c.Assert(err, gc.ErrorMatches, "client error!")
	AssertEventuallyEqual(c, &t.numCalls, 1)
	c.Assert(m, gc.IsNil)
}

func (s *CleanerSuite) TestWatchCleanupsFailFacadeResult(c *gc.C) {
	e := params.Error{
		Message: "Server Error",
	}
	p := params.NotifyWatchResult{
		Error: &e,
	}
	t := Init(c, "", nil, p, nil)
	m, err := t.api.WatchCleanups()
	AssertEventuallyEqual(c, &t.numCalls, 1)
	c.Assert(err, gc.ErrorMatches, e.Message)
	c.Assert(m, gc.IsNil)
}
