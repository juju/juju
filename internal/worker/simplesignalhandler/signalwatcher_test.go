// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package simplesignalhandler_test

import (
	"fmt"
	"os"
	"syscall"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	ssh "github.com/juju/juju/internal/worker/simplesignalhandler"
)

type signalSuite struct {
}

func TestSignalSuite(t *testing.T) {
	tc.Run(t, &signalSuite{})
}

func (*signalSuite) TestSignalHandling(c *tc.C) {
	testErr := errors.ConstError("test")
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return testErr
	})

	sigChan := make(chan os.Signal)

	watcher, err := ssh.NewSignalWatcher(loggertesting.WrapCheckLog(c), sigChan, handler)
	c.Assert(err, tc.ErrorIsNil)

	sigChan <- syscall.SIGTERM

	err = watcher.Wait()
	c.Assert(err, tc.ErrorIs, testErr)
}

func (*signalSuite) TestSignalHandlingClosed(c *tc.C) {
	handler := ssh.SignalHandlerFunc(func(sig os.Signal) error {
		return fmt.Errorf("should not be called")
	})

	sigChan := make(chan os.Signal)

	watcher, err := ssh.NewSignalWatcher(loggertesting.WrapCheckLog(c), sigChan, handler)
	c.Assert(err, tc.ErrorIsNil)

	close(sigChan)

	err = watcher.Wait()
	c.Assert(err.Error(), tc.Equals, "signal channel closed unexpectedly")
}

func (*signalSuite) TestDefaultSignalHandlerNilMap(c *tc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, nil)(syscall.SIGTERM)
	c.Assert(err, tc.ErrorIs, testErr)
}

func (*signalSuite) TestDefaultSignalHandlerNoMap(c *tc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGTERM)
	c.Assert(err, tc.ErrorIs, testErr)
}

func (*signalSuite) TestDefaultSignalHandlerMap(c *tc.C) {
	testErr := errors.ConstError("test")
	err := ssh.SignalHandler(testErr, map[os.Signal]error{
		syscall.SIGINT: errors.New("test error"),
	})(syscall.SIGINT)
	c.Assert(err, tc.Not(tc.ErrorIs), testErr)
	c.Assert(err.Error(), tc.Equals, "test error")
}
