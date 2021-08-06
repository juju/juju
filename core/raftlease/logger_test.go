// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"io"
	"strings"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type LoggerSuite struct{}

var _ = gc.Suite(&LoggerSuite{})

func (LoggerSuite) TestInfo(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var called bool

	leaseLogger := NewMockWriter(ctrl)
	leaseLogger.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		called = true
		c.Assert(strings.Contains(string(b), "boom"), jc.IsTrue)
		return len(b), nil
	})

	errorLogger := NewMockErrorLogger(ctrl)

	logger := NewTargetLogger(leaseLogger, errorLogger)
	logger.Infof("boom")

	c.Assert(called, jc.IsTrue)
}

func (LoggerSuite) TestInfoFallbackToErrorLogger(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var called bool

	leaseLogger := NewMockWriter(ctrl)
	leaseLogger.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		called = true
		c.Assert(strings.Contains(string(b), "boom"), jc.IsTrue)
		return len(b), io.EOF
	})

	errorLogger := NewMockErrorLogger(ctrl)
	errorLogger.EXPECT().Errorf("couldn't write to lease log with messags %q: %s", "boom", "EOF")

	logger := NewTargetLogger(leaseLogger, errorLogger)
	logger.Infof("boom")

	c.Assert(called, jc.IsTrue)
}

func (LoggerSuite) TestError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	var called bool

	leaseLogger := NewMockWriter(ctrl)
	leaseLogger.EXPECT().Write(gomock.Any()).DoAndReturn(func(b []byte) (int, error) {
		called = true
		c.Assert(strings.Contains(string(b), "boom"), jc.IsTrue)
		return len(b), nil
	})

	errorLogger := NewMockErrorLogger(ctrl)
	errorLogger.EXPECT().Errorf("boom")

	logger := NewTargetLogger(leaseLogger, errorLogger)
	logger.Errorf("boom")

	c.Assert(called, jc.IsTrue)
}
