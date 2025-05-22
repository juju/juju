// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package querylogger

import (
	"fmt"
	"os"
	"path/filepath"
	stdtesting "testing"
	time "time"

	"github.com/juju/tc"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/internal/testhelpers"
)

type loggerSuite struct {
	testhelpers.IsolationSuite

	clock  *MockClock
	timer  *MockTimer
	logger *MockLogger
}

func TestLoggerSuite(t *stdtesting.T) {
	tc.Run(t, &loggerSuite{})
}

func (s *loggerSuite) TestLogger(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()

	w := s.newWorker(c, dir)
	defer workertest.DirtyKill(c, w)

	args := []any{0.1, "SELECT * FROM foo"}
	s.logger.EXPECT().Warningf(gomock.Any(), "slow query: hello", args)

	w.RecordSlowQuery("hello", "SELECT * FROM foo", args, 0.1)

	select {
	case ch <- time.Now():
	case <-time.After(testhelpers.ShortWait):
		c.Fatal("timed out waiting for log to be written")
	}

	s.expectLogResult(c, dir, `
slow query took 0.100s for statement: SELECT * FROM foo
stack trace:
dummy stack

`[1:])

	workertest.CleanKill(c, w)
}

func (s *loggerSuite) TestLoggerMultipleTimes(c *tc.C) {
	defer s.setupMocks(c).Finish()

	dir := c.MkDir()

	ch := make(chan time.Time)
	s.timer.EXPECT().Chan().Return(ch).AnyTimes()

	w := s.newWorker(c, dir)
	defer workertest.DirtyKill(c, w)

	for i := 0; i < 100; i++ {
		stmt := fmt.Sprintf("SELECT %d FROM foo", i)
		args := []any{i, stmt}

		s.logger.EXPECT().Warningf(gomock.Any(), "slow query: hello", args)

		w.RecordSlowQuery("hello", stmt, args, float64(i))
	}

	select {
	case ch <- time.Now():
	case <-time.After(testhelpers.ShortWait):
		c.Fatal("timed out waiting for log to be written")
	}

	template := `
slow query took %0.3fs for statement: SELECT %d FROM foo
stack trace:
dummy stack

`[1:]

	var expected string
	for i := 0; i < 100; i++ {
		expected += fmt.Sprintf(template, float64(i), i)
	}

	s.expectLogResult(c, dir, expected)

	workertest.CleanKill(c, w)
}

func (s *loggerSuite) expectLogResult(c *tc.C, dir string, match string) {
	data, err := os.ReadFile(filepath.Join(dir, filename))
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(string(data), tc.Equals, match)
}

func (s *loggerSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.timer = NewMockTimer(ctrl)
	s.timer.EXPECT().Reset(PollInterval)
	s.timer.EXPECT().Stop()

	s.clock = NewMockClock(ctrl)
	s.clock.EXPECT().NewTimer(PollInterval).Return(s.timer)

	s.logger = NewMockLogger(ctrl)

	return ctrl
}

func (s *loggerSuite) newWorker(c *tc.C, dir string) *loggerWorker {
	w, err := newWorker(&WorkerConfig{
		LogDir: dir,
		Clock:  s.clock,
		Logger: s.logger,
		StackGatherer: func() []byte {
			return []byte("dummy stack")
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	return w
}
