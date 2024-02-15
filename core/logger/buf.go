// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"sync"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
)

// BufferedLogger wraps a Logger, providing a buffer that
// accumulates log messages, flushing them to the underlying logger
// when enough messages have been accumulated.
type BufferedLogger struct {
	l             Logger
	clock         clock.Clock
	flushInterval time.Duration

	mu         sync.Mutex
	buf        []LogRecord
	flushTimer clock.Timer
}

// NewBufferedLogger returns a new BufferedLogger, wrapping the given
// Logger with a buffer of the specified size and flush interval.
func NewBufferedLogger(
	l Logger,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock,
) *BufferedLogger {
	return &BufferedLogger{
		l:             l,
		buf:           make([]LogRecord, 0, bufferSize),
		clock:         clock,
		flushInterval: flushInterval,
	}
}

// Log is part of the Logger interface.
//
// BufferedLogger's Log implementation will buffer log records up to
// the specified capacity and duration; after either of which is exceeded,
// the records will be flushed to the underlying logger.
// TODO(debug-log) - we may receive log messages slightly out of order.
// We should ensure they are sorted on timestamp.
func (b *BufferedLogger) Log(in []LogRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for len(in) > 0 {
		r := cap(b.buf) - len(b.buf)
		n := len(in)
		if n > r {
			n = r
		}
		b.buf = append(b.buf, in[:n]...)
		in = in[n:]
		if len(b.buf) >= cap(b.buf) {
			if err := b.flush(); err != nil {
				return errors.Trace(err)
			}
		}
	}
	if len(b.buf) > 0 && b.flushTimer == nil {
		b.flushTimer = b.clock.AfterFunc(b.flushInterval, b.flushOnTimer)
	}
	return nil
}

// Flush flushes any buffered log records to the underlying Logger.
func (b *BufferedLogger) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

func (b *BufferedLogger) flushOnTimer() {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Can't do anything about errors here, except to
	// ignore them and let the Log() method report them
	// when the buffer fills up.
	b.flush()
}

// flush flushes any buffered log records to the underlying Logger, and stops
// the flush timer if there is one. The caller must be holding b.mu.
func (b *BufferedLogger) flush() error {
	if b.flushTimer != nil {
		b.flushTimer.Stop()
		b.flushTimer = nil
	}
	if len(b.buf) > 0 {
		if err := b.l.Log(b.buf); err != nil {
			return errors.Trace(err)
		}
		b.buf = b.buf[:0]
	}
	return nil
}
