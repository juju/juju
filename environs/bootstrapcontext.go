// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environs

import (
	"io"
	"os"
	"os/signal"
	"sync"
)

// setInterruptHandlerElem is the element type of the
// handlerchan channel. It contains a new interrupt
// handler, and a channel to respond with the old
// handler.
type setInterruptHandlerElem struct {
	newhandler func()
	oldhandler chan func()
}

// BootstrapContext is a structure used to convey
// information about the context in which bootstrap
// methods are being invoked.
type BootstrapContext struct {
	once        sync.Once
	handlerchan chan setInterruptHandlerElem

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

// NewBootstrapContext returns a new BootstrapContext
// with Stdin, Stdout and Stderr initialised to their
// os namesakes.
func NewBootstrapContext() *BootstrapContext {
	return &BootstrapContext{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
}

// SetInterruptHandler sets a function that will be
// invoked when the process receives an interrupt
// (SIGINT) signal. If the handler specified is nil,
// signal handling will be disabled (the default).
func (ctx *BootstrapContext) SetInterruptHandler(f func()) (old func()) {
	ctx.once.Do(ctx.initHandler)
	oldhandler := make(chan func(), 1)
	ctx.handlerchan <- setInterruptHandlerElem{
		newhandler: f,
		oldhandler: oldhandler,
	}
	return <-oldhandler
}

func (ctx *BootstrapContext) initHandler() {
	ctx.handlerchan = make(chan setInterruptHandlerElem)
	go ctx.handleInterrupt()
}

// handlerInterrupt is responsible for keeping
// the handler and signal handling in sync. When
// handler goes from nil->non-nil, signal handling
// is enabled, and disabled on non-nil->nil.
func (ctx *BootstrapContext) handleInterrupt() {
	signalchan := make(chan os.Signal, 1)
	var s chan os.Signal
	var handler func()
	for {
		select {
		case elem := <-ctx.handlerchan:
			elem.oldhandler <- handler
			handler = elem.newhandler
			if handler == nil {
				if s != nil {
					signal.Stop(signalchan)
					s = nil
				}
			} else {
				if s == nil {
					s = signalchan
					signal.Notify(signalchan, os.Interrupt)
				}
			}
		case <-s:
			handler()
		}
	}
}
