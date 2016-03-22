// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder

import (
	"sync/atomic"
)

// stringForwarder will take messages on a receive() method and forward them to
// callback, but will drop them if callback() has not finished processing the
// last message. We will track how many messages we have discarded.
type stringForwarder struct {
	callback     func(string)
	buffer       chan string
	stopch       chan struct{}
	started      chan struct{}
	discardCount uint64
}

func NewStringForwarder(callback func(string)) *stringForwarder {
	if callback == nil {
		// Nothing to forward to, so no need to start the loop(). We'll
		// just count the discardCount.
		return &stringForwarder{}
	}
	forwarder := &stringForwarder{
		callback:     callback,
		buffer:       make(chan string),
		stopch:       make(chan struct{}),
		discardCount: 0,
	}
	started := make(chan struct{})
	go forwarder.loop(started)
	<-started
	return forwarder
}

func (f *stringForwarder) Receive(msg string) {
	select {
	case f.buffer <- msg:
	default:
		atomic.AddUint64(&f.discardCount, 1)
	}
}

func (f *stringForwarder) Stop() uint64 {
	if f.stopch != nil {
		close(f.stopch)
		f.stopch = nil
	}
	return atomic.LoadUint64(&f.discardCount)
}

func (f *stringForwarder) loop(started chan struct{}) {
	stopch := f.stopch
	close(started)
	for {
		select {
		case msg := <-f.buffer:
			f.callback(msg)
		case <-stopch:
			return
		}
	}
}
