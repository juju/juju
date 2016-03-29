// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package stringforwarder

import (
	"sync"
	"sync/atomic"
)

// StringForwarder is a goroutine-safe type that pipes messages from the
// its Forward() method, sending them to callback.  The send is non-blocking and
// will discard messages if the last message has not finished processing.
// The number of discarded messages is tracked and returned when the forwarder
// is stopped.
type StringForwarder struct {
	messages     chan string
	done         chan struct{}
	discardCount uint64
	mu           *sync.Mutex
}

// New returns a new StringForwarder that sends messages to the callback,
// function, dropping messages if the receiver is not ready.
func New(callback func(string)) *StringForwarder {
	if callback == nil {
		// Nothing to forward to, so no need to start the loop(). We'll
		// just count the discardCount.
		return &StringForwarder{mu: &sync.Mutex{}}
	}

	messages := make(chan string)
	done := make(chan struct{})
	forwarder := &StringForwarder{
		messages:     messages,
		done:         done,
		mu:           &sync.Mutex{},
		discardCount: 0,
	}
	started := make(chan struct{})
	go forwarder.loop(started, callback)
	<-started
	return forwarder
}

// Forward makes a non-blocking send of the message to the callback function.
// If the message is not received, it will increment the count of discarded
// messages. Forward is safe to call from multiple goroutines at once.
// Note that if this StringForwarder was created with a nil callback, all
// messages will be discarded.
func (f *StringForwarder) Forward(msg string) {
	select {
	case f.messages <- msg:
	default:
		atomic.AddUint64(&f.discardCount, 1)
	}
}

// Stop cleans up the goroutine running behind StringForwarder and returns the
// count of discarded messages. Stop is thread-safe and may be called multiple
// times - after the first time, it simply returns the current discard count.
func (f *StringForwarder) Stop() uint64 {
	f.mu.Lock()
	if f.done != nil {
		close(f.done)
		f.done = nil
	}
	f.mu.Unlock()
	return atomic.LoadUint64(&f.discardCount)
}

// loop pipes messages from the messages channel into the callback function. It
// closes started to signal that it has begun, and will clean itself up when
// done is closed.
func (f *StringForwarder) loop(started chan struct{}, callback func(string)) {
	close(started)
	for {
		select {
		case msg := <-f.messages:
			callback(msg)
		case <-f.done:
			return
		}
	}
}
