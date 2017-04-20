// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"os"
	"os/signal"

	gc "gopkg.in/check.v1"
)

// SignalSuite inserts a signal handler to catch shutdown signals
// and run the supplied function in its place.
type SignalSuite struct {

	// ch holds the channel registered with the signal handler.
	ch chan os.Signal

	// quit holds the channel used to tell handleSignal to exit.
	quit chan bool
}

// SetUpSuite registers sigfunc to be called when a signal is received.
// As a terminating signal is caught at this point, sigfunc should cause the
// process to exit. sigfunc should not return.
func (s *SignalSuite) SetUpSuite(c *gc.C, sigfunc func(*gc.C, os.Signal)) {
	if sigfunc == nil {
		c.Fatal("sigfunc cannot be nil")
	}

	s.ch = make(chan os.Signal, 1)
	s.quit = make(chan bool)
	go s.handleSignal(c, sigfunc)
	c.Logf("registering handler for: %s", Signals)
	signal.Notify(s.ch, Signals...)
}

func (s *SignalSuite) TearDownSuite(c *gc.C) {
	// deregister s.ch and close channel to exit handleSignal worker.
	signal.Stop(s.ch)

	// even though we asked os/signal to stop sending signals to this channel
	// it appear that it still holds a reference to the channel and if closed
	// the program will dump core. Instead of closing s.ch to signal handleSignal
	// to quit, we close s.quit instead.
	close(s.quit)
	c.Logf("stopped listening for: %s", Signals)
}

func (s *SignalSuite) handleSignal(c *gc.C, sigfunc func(*gc.C, os.Signal)) {
	c.Logf("signal handler waiting for: %s", Signals)
	for {
		select {
		case sig := <-s.ch:
			sigfunc(c, sig)

			// sigfunc is expected to call os.Exit, it should not return.
			c.Errorf("sigfunc should not have returned")
		case <-s.quit:

			// during TearDownSuite we unregister the channel for signal notifications
			// and close s.quit, this will cause the range loop to exit, shutting down this
			// goroutine.
			c.Logf("signal handler exited")
		}
	}
}
