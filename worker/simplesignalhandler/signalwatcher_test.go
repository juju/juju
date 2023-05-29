// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplesignalhandler_test

import (
	"fmt"
	"os"
	"syscall"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	. "gopkg.in/check.v1"

	ssh "github.com/juju/juju/worker/simplesignalhandler"
)

type signalSuite struct {
}

var _ = Suite(&signalSuite{})

func (_ *signalSuite) TestSignalHandling(c *C) {
	testErr := errors.ConstError("test")
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return testErr
	})

	sigChan := make(chan os.Signal, 0)

	watcher, err := ssh.NewSignalWatcher(loggo.Logger{}, sigChan, handler)
	c.Assert(err, jc.ErrorIsNil)

	sigChan <- syscall.SIGTERM

	err = watcher.Wait()
	c.Assert(errors.Is(err, testErr), jc.IsTrue)
}

func (_ *signalSuite) TestSignalHandlingClosed(c *C) {
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return fmt.Errorf("should not be called")
	})

	sigChan := make(chan os.Signal, 0)

	watcher, err := ssh.NewSignalWatcher(loggo.Logger{}, sigChan, handler)
	c.Assert(err, jc.ErrorIsNil)

	close(sigChan)

	err = watcher.Wait()
	c.Assert(err.Error(), Equals, "signal channel closed unexpectedly")
}

func (_ *signalSuite) TestDefaultSignalHandlerNilMap(c *C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, nil)(syscall.SIGTERM)
	c.Assert(errors.Is(err, testErr), jc.IsTrue)
}

func (_ *signalSuite) TestDefaultSignalHandlerNoMap(c *C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGTERM)
	c.Assert(errors.Is(err, testErr), jc.IsTrue)
}

func (_ *signalSuite) TestDefaultSignalHandlerMap(c *C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGINT)
	c.Assert(errors.Is(err, testErr), jc.IsFalse)
	c.Assert(err.Error(), Equals, "test error")
}
