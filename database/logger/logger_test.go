// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"fmt"
	"os"
	"path/filepath"
	time "time"

	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type loggerSuite struct {
	testing.IsolationSuite

	clock *MockClock
	timer *MockTimer
}

var _ = gc.Suite(&loggerSuite{})

func (s *loggerSuite) TestLogger(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()

	logger := NewSlowQueryLogger(dir, s.clock, stubLogger{})

	err := logger.Log("hello", 0.1, "SELECT * FROM foo", []byte("dummy stack"))
	c.Assert(err, jc.ErrorIsNil)

	select {
	case ch <- time.Now():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for log to be written")
	}

	s.expectLogResult(c, dir, `
slow query took 0.100s for statement: SELECT * FROM foo
stack trace:
dummy stack

`[1:])

	err = logger.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loggerSuite) TestLoggerMultipleTimes(c *gc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()

	logger := NewSlowQueryLogger(dir, s.clock, stubLogger{})

	for i := 0; i < 100; i++ {
		stack := fmt.Sprintf("dummy stack\n%d", i)
		err := logger.Log(fmt.Sprintf("hello %d", i), float64(i), fmt.Sprintf("SELECT %d FROM foo", i), []byte(stack))
		c.Assert(err, jc.ErrorIsNil)
	}

	select {
	case ch <- time.Now():
	case <-time.After(testing.ShortWait):
		c.Fatal("timed out waiting for log to be written")
	}

	template := `
slow query took %0.3fs for statement: SELECT %d FROM foo
stack trace:
dummy stack
%d

`[1:]

	var expected string
	for i := 0; i < 100; i++ {
		expected += fmt.Sprintf(template, float64(i), i, i)
	}

	s.expectLogResult(c, dir, expected)

	err := logger.Close()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loggerSuite) expectLogResult(c *gc.C, dir string, match string) {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(data), gc.Equals, match)
}

func (s *loggerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.timer = NewMockTimer(ctrl)
	s.timer.EXPECT().Reset(PollInterval)
	s.timer.EXPECT().Stop()

	s.clock = NewMockClock(ctrl)
	s.clock.EXPECT().NewTimer(PollInterval).Return(s.timer)

	return ctrl
}
