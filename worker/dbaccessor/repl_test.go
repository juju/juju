// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package dbaccessor

import (
	"path/filepath"

	"github.com/juju/loggo"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/workertest"
	gc "gopkg.in/check.v1"
)

type REPLSuite struct{}

var _ = gc.Suite(&REPLSuite{})

func (s *REPLSuite) TestREPL(c *gc.C) {
	dir := c.MkDir()
	path := filepath.Join(dir, "socket")

	repl, err := newREPL(path, nil, clock.WallClock, fakeLogger{})
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, repl)
}

type fakeLogger struct{}

func (fakeLogger) Errorf(_ string, _ ...interface{})        {}
func (fakeLogger) Warningf(_ string, _ ...interface{})      {}
func (fakeLogger) Infof(_ string, _ ...interface{})         {}
func (fakeLogger) Debugf(_ string, _ ...interface{})        {}
func (fakeLogger) Tracef(_ string, _ ...interface{})        {}
func (fakeLogger) Logf(loggo.Level, string, ...interface{}) {}
func (fakeLogger) IsTraceEnabled() bool                     { return false }
