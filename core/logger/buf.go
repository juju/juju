// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logger

import (
	"sort"
	"sync"
	"time"

	"github.com/juju/clock"

	"github.com/juju/juju/internal/errors"
)

// BufferedLogWriter wraps a LogWriter, providing a buffer that
// accumulates log messages, flushing them to the underlying logger
// when enough messages have been accumulated.
// The emitted records are sorted by timestamp.
type BufferedLogWriter struct {
	l             LogWriter
	clock         clock.Clock
	flushInterval time.Duration

	mu         sync.Mutex
	buf        []LogRecord
	flushTimer clock.Timer
}

// NewBufferedLogWriter returns a new BufferedLogWriter, wrapping the given
// Logger with a buffer of the specified size and flush interval.
func NewBufferedLogWriter(
	l LogWriter,
	bufferSize int,
	flushInterval time.Duration,
	clock clock.Clock,
) *BufferedLogWriter {
	return &BufferedLogWriter{
		l:             l,
		buf:           make([]LogRecord, 0, bufferSize),
		clock:         clock,
		flushInterval: flushInterval,
	}
}

func insertSorted(recs []LogRecord, in []LogRecord) []LogRecord {
	for _, r := range in {
		i := sort.Search(len(recs), func(i int) bool { return recs[i].Time.After(r.Time) })
		if len(recs) == i {
			recs = append(recs, r)
			continue
		}
		recs = append(recs[:i+1], recs[i:]...)
		recs[i] = r
	}
	return recs
}

// Log is part of the Logger interface.
//
// BufferedLogWriter's Log implementation will buffer log records up to
// the specified capacity and duration; after either of which is exceeded,
// the records will be flushed to the underlying logger.
func (b *BufferedLogWriter) Log(in []LogRecord) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Sort incoming records.
	// We only use the first N so they need
	// to be sorted first up.
	sort.Slice(in, func(i, j int) bool {
		return in[i].Time.Before(in[j].Time)
	})

	for len(in) > 0 {
		r := cap(b.buf) - len(b.buf)
		n := len(in)
		if n > r {
			n = r
		}
		b.buf = insertSorted(b.buf, in[:n])
		in = in[n:]
		if len(b.buf) >= cap(b.buf) {
			if err := b.flush(); err != nil {
				return errors.Capture(err)
			}
		}
	}
	if len(b.buf) > 0 && b.flushTimer == nil {
		b.flushTimer = b.clock.AfterFunc(b.flushInterval, b.flushOnTimer)
	}
	return nil
}

// Flush flushes any buffered log records to the underlying Logger.
func (b *BufferedLogWriter) Flush() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.flush()
}

func (b *BufferedLogWriter) flushOnTimer() {
	b.mu.Lock()
	defer b.mu.Unlock()
	// Can't do anything about errors here, except to
	// ignore them and let the Log() method report them
	// when the buffer fills up.
	b.flush()
}

// flush flushes any buffered log records to the underlying Logger, and stops
// the flush timer if there is one. The caller must be holding b.mu.
func (b *BufferedLogWriter) flush() error {
	if b.flushTimer != nil {
		b.flushTimer.Stop()
		b.flushTimer = nil
	}
	if len(b.buf) > 0 {
		if err := b.l.Log(b.buf); err != nil {
			return errors.Capture(err)
		}
		b.buf = b.buf[:0]
	}
	return nil
}
