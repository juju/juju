// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package logsender

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/juju/collections/deque"
	"github.com/juju/errors"
	"github.com/juju/loggo"
)

// LogRecord represents a log message in an agent which is to be
// transmitted to the JES.
type LogRecord struct {
	Time     time.Time
	Module   string
	Location string // e.g. "foo.go:42"
	Level    loggo.Level
	Message  string
	Labels   []string

	// Number of messages dropped after this one due to buffer limit.
	DroppedAfter int
}

// LogStats contains statistics on logging.
type LogStats struct {
	// Enqueued is the number of log messages enqueued.
	Enqueued uint64

	// Sent is the number of log messages sent.
	Sent uint64

	// Dropped is the number of log messages dropped from the queue.
	Dropped uint64
}

// LogRecordCh defines the channel type used to send log message
// structs within the unit and machine agents.
type LogRecordCh chan *LogRecord

const writerName = "buffered-logs"

// InstallBufferedLogWriter creates and returns a new BufferedLogWriter,
// registering it with Loggo.
func InstallBufferedLogWriter(context *loggo.Context, maxLen int) (*BufferedLogWriter, error) {
	writer := NewBufferedLogWriter(maxLen)
	err := context.AddWriter(writerName, writer)
	if err != nil {
		return nil, errors.Annotate(err, "failed to set up log buffering")
	}
	return writer, nil
}

// UninstallBufferedLogWriter removes the BufferedLogWriter previously
// installed by InstallBufferedLogWriter and closes it.
func UninstallBufferedLogWriter() error {
	writer, err := loggo.RemoveWriter(writerName)
	if err != nil {
		return errors.Annotate(err, "failed to uninstall log buffering")
	}
	bufWriter, ok := writer.(*BufferedLogWriter)
	if !ok {
		return errors.New("unexpected writer installed as buffered log writer")
	}
	bufWriter.Close()
	return nil
}

// BufferedLogWriter is a loggo.Writer which buffers log messages in
// memory. These messages are retrieved by reading from the channel
// returned by the Logs method.
//
// Up to maxLen log messages will be buffered. If this limit is
// exceeded, the oldest records will be automatically discarded.
type BufferedLogWriter struct {
	maxLen int
	in     LogRecordCh
	out    LogRecordCh

	mu    sync.Mutex
	stats LogStats
}

// NewBufferedLogWriter returns a new BufferedLogWriter which will
// cache up to maxLen log messages.
func NewBufferedLogWriter(maxLen int) *BufferedLogWriter {
	w := &BufferedLogWriter{
		maxLen: maxLen,
		in:     make(LogRecordCh),
		out:    make(LogRecordCh),
	}
	go w.loop()
	return w
}

func (w *BufferedLogWriter) loop() {
	buffer := deque.New()
	var outCh LogRecordCh // Output channel - set when there's something to send.
	var outRec *LogRecord // Next LogRecord to send to the output channel.

	for {
		// If there's something in the buffer and there's nothing
		// queued up to send, set up the next LogRecord to send.
		if outCh == nil {
			if item, haveItem := buffer.PopFront(); haveItem {
				outRec = item.(*LogRecord)
				outCh = w.out
			}
		}

		select {
		case inRec, ok := <-w.in:
			if !ok {
				// Input channel has been closed; finish up.
				close(w.out)
				return
			}

			buffer.PushBack(inRec)

			w.mu.Lock()
			w.stats.Enqueued++
			if buffer.Len() > w.maxLen {
				// The buffer has exceeded the limit - discard the
				// next LogRecord from the front of the queue.
				buffer.PopFront()
				outRec.DroppedAfter++
				w.stats.Dropped++
			}
			w.mu.Unlock()

		case outCh <- outRec:
			outCh = nil // Signal that send happened.

			w.mu.Lock()
			w.stats.Sent++
			w.mu.Unlock()
		}
	}
}

// Write sends a new log message to the writer.
// This implements the loggo.Writer interface.
func (w *BufferedLogWriter) Write(entry loggo.Entry) {
	w.in <- &LogRecord{
		Time:     entry.Timestamp,
		Module:   entry.Module,
		Location: fmt.Sprintf("%s:%d", filepath.Base(entry.Filename), entry.Line),
		Level:    entry.Level,
		Message:  entry.Message,
		Labels:   entry.Labels,
	}
}

// Logs returns a channel which emits log messages that have been sent
// to the BufferedLogWriter instance.
func (w *BufferedLogWriter) Logs() LogRecordCh {
	return w.out
}

// Capacity returns the capacity of the BufferedLogWriter.
func (w *BufferedLogWriter) Capacity() int {
	return w.maxLen
}

// Stats returns the current LogStats for this BufferedLogWriter.
func (w *BufferedLogWriter) Stats() LogStats {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.stats
}

// Close cleans up the BufferedLogWriter instance. The output channel
// returned by the Logs method will be closed and any further Write
// calls will panic.
func (w *BufferedLogWriter) Close() {
	close(w.in)
}
