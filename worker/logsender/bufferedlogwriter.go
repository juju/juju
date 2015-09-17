// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENSE file for details.

package logsender

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/juju/errors"
	"github.com/juju/juju/feature"
	"github.com/juju/loggo"
	"github.com/juju/utils/deque"
)

// LogRecord represents a log message in an agent which is to be
// transmitted to the JES.
type LogRecord struct {
	Time     time.Time
	Module   string
	Location string // e.g. "foo.go:42"
	Level    loggo.Level
	Message  string

	// Number of messages dropped after this one due to buffer limit.
	DroppedAfter int
}

// LogRecordCh defines the channel type used to send log message
// structs within the unit and machine agents.
type LogRecordCh chan *LogRecord

const writerName = "buffered-logs"

// InstallBufferedLogWriter creates a new BufferedLogWriter, registers
// it with Loggo and returns its output channel.
func InstallBufferedLogWriter(maxLen int) (LogRecordCh, error) {
	if !feature.IsDbLogEnabled() {
		return nil, nil
	}

	writer := NewBufferedLogWriter(maxLen)
	err := loggo.RegisterWriter(writerName, writer, loggo.TRACE)
	if err != nil {
		return nil, errors.Annotate(err, "failed to set up log buffering")
	}
	return writer.Logs(), nil
}

// UninstallBufferedLogWriter removes the BufferedLogWriter previously
// installed by InstallBufferedLogWriter and closes it.
func UninstallBufferedLogWriter() error {
	if !feature.IsDbLogEnabled() {
		return nil
	}

	writer, _, err := loggo.RemoveWriter(writerName)
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

			if buffer.Len() > w.maxLen {
				// The buffer has exceeded the limit - discard the
				// next LogRecord from the front of the queue.
				buffer.PopFront()
				outRec.DroppedAfter++
			}

		case outCh <- outRec:
			outCh = nil // Signal that send happened.
		}
	}

}

// Write sends a new log message to the writer. This implements the loggo.Writer interface.
func (w *BufferedLogWriter) Write(level loggo.Level, module, filename string, line int, ts time.Time, message string) {
	w.in <- &LogRecord{
		Time:     ts,
		Module:   module,
		Location: fmt.Sprintf("%s:%d", filepath.Base(filename), line),
		Level:    level,
		Message:  message,
	}
}

// Logs returns a channel which emits log messages that have been sent
// to the BufferedLogWriter instance.
func (w *BufferedLogWriter) Logs() LogRecordCh {
	return w.out
}

// Close cleans up the BufferedLogWriter instance. The output channel
// returned by the Logs method will be closed and any further Write
// calls will panic.
func (w *BufferedLogWriter) Close() {
	close(w.in)
}
