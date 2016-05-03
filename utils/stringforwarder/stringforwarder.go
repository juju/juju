// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder

import "sync"

// StringForwarder is a goroutine-safe type that pipes messages from the
// its Forward() method, sending them to callback.  The send will not be
// blocked by the callback, but will instead discard messages if there
// is an incomplete callback in progress. The number of discarded messages
// is tracked and returned when the forwarder is stopped.
type StringForwarder struct {
	mu           sync.Mutex
	cond         *sync.Cond
	current      *string
	stopped      bool
	discardCount uint64
}

// New returns a new StringForwarder that sends messages to the callback,
// function, dropping messages if the receiver has not yet consumed the
// last message.
func New(callback func(string)) *StringForwarder {
	if callback == nil {
		// Nothing to forward to, so no need to start the loop().
		// We'll just count the discardCount.
		return &StringForwarder{stopped: true}
	}
	forwarder := &StringForwarder{}
	forwarder.cond = sync.NewCond(&forwarder.mu)
	go forwarder.loop(callback)
	return forwarder
}

// Forward sends the message to be processed by the callback function,
// discarding the message if another message is currently being processed.
// The number of discarded messages is recorded for reporting by the Stop
// method.
//
// Forward is safe to call from multiple goroutines at once.
// Note that if this StringForwarder was created with a nil callback, all
// messages will be discarded.
func (f *StringForwarder) Forward(msg string) {
	f.mu.Lock()
	if f.stopped || f.current != nil {
		f.discardCount++
	} else {
		f.current = &msg
		f.cond.Signal()
	}
	f.mu.Unlock()
}

// Stop cleans up the goroutine running behind StringForwarder and returns the
// count of discarded messages. Stop is thread-safe and may be called multiple
// times - after the first time, it simply returns the current discard count.
func (f *StringForwarder) Stop() uint64 {
	var count uint64
	f.mu.Lock()
	if !f.stopped {
		f.stopped = true
		f.cond.Signal()
	}
	count = f.discardCount
	f.mu.Unlock()
	return count
}

// loop invokes forwarded messages with the given callback until stopped.
func (f *StringForwarder) loop(callback func(string)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for {
		for !f.stopped && f.current == nil {
			f.cond.Wait()
		}
		if f.stopped {
			return
		}
		f.invokeCallback(callback, *f.current)
		f.current = nil
	}
}

// invokeCallback invokes the given callback with a message,
// unlocking the forwarder's mutex for the duration of the
// callback.
func (f *StringForwarder) invokeCallback(callback func(string), msg string) {
	f.mu.Unlock()
	defer f.mu.Lock()
	callback(msg)
}
