// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplesignalhandler_test

import (
	"fmt"
	"os"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo/v2"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	ssh "github.com/juju/juju/internal/worker/simplesignalhandler"
)

type signalSuite struct {
}

var _ = gc.Suite(&signalSuite{})

func (_ *signalSuite) TestSignalHandling(c *gc.C) {
	testErr := errors.ConstError("test")
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return testErr
	})

	sigChan := make(chan os.Signal, 0)

	watcher, err := ssh.NewSignalWatcher(loggo.Logger{}, sigChan, handler)
	c.Assert(err, jc.ErrorIsNil)

	sigChan <- syscall.SIGTERM

	err = watcher.Wait()
	c.Assert(err, jc.ErrorIs, testErr)
}

func (_ *signalSuite) TestSignalHandlingClosed(c *gc.C) {
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return fmt.Errorf("should not be called")
	})

	sigChan := make(chan os.Signal, 0)

	watcher, err := ssh.NewSignalWatcher(loggo.Logger{}, sigChan, handler)
	c.Assert(err, jc.ErrorIsNil)

	close(sigChan)

	err = watcher.Wait()
	c.Assert(err.Error(), gc.Equals, "signal channel closed unexpectedly")
}

func (_ *signalSuite) TestDefaultSignalHandlerNilMap(c *gc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, nil)(syscall.SIGTERM)
	c.Assert(err, jc.ErrorIs, testErr)
}

func (_ *signalSuite) TestDefaultSignalHandlerNoMap(c *gc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGTERM)
	c.Assert(err, jc.ErrorIs, testErr)
}

func (_ *signalSuite) TestDefaultSignalHandlerMap(c *gc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGINT)
	c.Assert(err, gc.Not(jc.ErrorIs), testErr)
	c.Assert(err.Error(), gc.Equals, "test error")
}
