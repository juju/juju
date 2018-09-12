// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package status_test

import (
	"errors"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/cmd/cmdtesting"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/juju/status"
	"github.com/juju/juju/testing"
)

type MinimalStatusSuite struct {
	testing.BaseSuite

	api   *fakeStatusAPI
	clock *timeRecorder
}

var _ = gc.Suite(&MinimalStatusSuite{})

func (s *MinimalStatusSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.api = &fakeStatusAPI{
		result: &params.FullStatus{
			Model: params.ModelStatusInfo{
				Name:     "test",
				CloudTag: "cloud-foo",
			},
		},
	}
	s.clock = &timeRecorder{}
	s.SetModelAndController(c, "test", "admin/test")
}

func (s *MinimalStatusSuite) runStatus(c *gc.C, args ...string) (*cmd.Context, error) {
	statusCmd := status.NewTestStatusCommand(s.api, s.clock)
	return cmdtesting.RunCommand(c, statusCmd, args...)
}

func (s *MinimalStatusSuite) TestGoodCall(c *gc.C) {
	_, err := s.runStatus(c)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(s.clock.waits, gc.HasLen, 0)
}

func (s *MinimalStatusSuite) TestRetryOnError(c *gc.C) {
	s.api.errors = []error{
		errors.New("boom"),
		errors.New("splat"),
	}

	_, err := s.runStatus(c)
	c.Assert(err, jc.ErrorIsNil)
	delay := 100 * time.Millisecond
	// Two delays of the default time.
	c.Assert(s.clock.waits, jc.DeepEquals, []time.Duration{delay, delay})
}

func (s *MinimalStatusSuite) TestRetryDelays(c *gc.C) {
	s.api.errors = []error{
		errors.New("boom"),
		errors.New("splat"),
	}

	_, err := s.runStatus(c, "--retry-delay", "250ms")
	c.Assert(err, jc.ErrorIsNil)
	delay := 250 * time.Millisecond
	c.Assert(s.clock.waits, jc.DeepEquals, []time.Duration{delay, delay})
}

func (s *MinimalStatusSuite) TestRetryCount(c *gc.C) {
	s.api.errors = []error{
		errors.New("error 1"),
		errors.New("error 2"),
		errors.New("error 3"),
		errors.New("error 4"),
		errors.New("error 5"),
		errors.New("error 6"),
		errors.New("error 7"),
	}

	_, err := s.runStatus(c, "--retry-count", "5")
	c.Assert(err.Error(), gc.Equals, "error 6")
	// We expect five waits of the default duration.
	delay := 100 * time.Millisecond
	c.Assert(s.clock.waits, jc.DeepEquals, []time.Duration{delay, delay, delay, delay, delay})
}

func (s *MinimalStatusSuite) TestRetryCountOfZero(c *gc.C) {
	s.api.errors = []error{
		errors.New("error 1"),
		errors.New("error 2"),
		errors.New("error 3"),
	}

	_, err := s.runStatus(c, "--retry-count", "0")
	c.Assert(err.Error(), gc.Equals, "error 1")
	// No delays.
	c.Assert(s.clock.waits, gc.HasLen, 0)
}

type fakeStatusAPI struct {
	result *params.FullStatus
	errors []error
}

func (f *fakeStatusAPI) Status(patterns []string) (*params.FullStatus, error) {
	if len(f.errors) > 0 {
		err, rest := f.errors[0], f.errors[1:]
		f.errors = rest
		return nil, err
	}
	return f.result, nil
}

func (*fakeStatusAPI) Close() error {
	return nil
}

type timeRecorder struct {
	waits  []time.Duration
	result chan time.Time
}

func (r *timeRecorder) After(d time.Duration) <-chan time.Time {
	r.waits = append(r.waits, d)
	if r.result == nil {
		// If we haven't yet, make a closed time channel so it immediately
		// passes.
		r.result = make(chan time.Time)
		close(r.result)
	}
	return r.result
}
