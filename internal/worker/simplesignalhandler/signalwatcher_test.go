// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplesignalhandler_test

import (
	"fmt"
	"os"
	"syscall"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	ssh "github.com/juju/juju/internal/worker/simplesignalhandler"
)

type signalSuite struct {
}

var _ = gc.Suite(&signalSuite{})

func (*signalSuite) TestSignalHandling(c *gc.C) {
	testErr := errors.ConstError("test")
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return testErr
	})

	sigChan := make(chan os.Signal)

	watcher, err := ssh.NewSignalWatcher(loggertesting.WrapCheckLog(c), sigChan, handler)
	c.Assert(err, jc.ErrorIsNil)

	sigChan <- syscall.SIGTERM

	err = watcher.Wait()
	c.Assert(err, jc.ErrorIs, testErr)
}

func (*signalSuite) TestSignalHandlingClosed(c *gc.C) {
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return fmt.Errorf("should not be called")
	})

	sigChan := make(chan os.Signal)

	watcher, err := ssh.NewSignalWatcher(loggertesting.WrapCheckLog(c), sigChan, handler)
	c.Assert(err, jc.ErrorIsNil)

	close(sigChan)

	err = watcher.Wait()
	c.Assert(err.Error(), gc.Equals, "signal channel closed unexpectedly")
}

func (*signalSuite) TestDefaultSignalHandlerNilMap(c *gc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, nil)(syscall.SIGTERM)
	c.Assert(err, jc.ErrorIs, testErr)
}

func (*signalSuite) TestDefaultSignalHandlerNoMap(c *gc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGTERM)
	c.Assert(err, jc.ErrorIs, testErr)
}

func (*signalSuite) TestDefaultSignalHandlerMap(c *gc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGINT)
	c.Assert(err, gc.Not(jc.ErrorIs), testErr)
	c.Assert(err.Error(), gc.Equals, "test error")
}
