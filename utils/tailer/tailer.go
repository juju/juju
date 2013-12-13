// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tailer

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"time"

	"launchpad.net/tomb"
)

const (
	bufferSize = 4096
	polltime   = time.Second
	delimiter  = '\n'
)

var (
	delimiters = []byte{delimiter}
)

// TailerFilterFunc decides if a line shall be tailed (func is nil or
// returns true) of shall be omitted (func returns false).
type TailerFilterFunc func(line []byte) bool

// Tailer reads an input line by line an tails them into the passed Writer.
// The lines have to be terminated with a newline.
type Tailer struct {
	tomb        tomb.Tomb
	readSeeker  io.ReadSeeker
	reader      *bufio.Reader
	writeCloser io.WriteCloser
	writer      *bufio.Writer
	lines       int
	filter      TailerFilterFunc
	bufferSize  int
	polltime    time.Duration
}

// NewTailer starts a Tailer which reads strings from the passed
// ReadSeeker line by line. If a filter function is specified the read
// lines are filtered. The matching lines are written to the passed
// Writer. The reading begins the specified number of matching lines
// from the end.
func NewTailer(readSeeker io.ReadSeeker, writer io.Writer, lines int, filter TailerFilterFunc) *Tailer {
	return newTailer(readSeeker, writer, lines, filter, bufferSize, polltime)
}

// newTailer starts a Tailer like NewTailer but allows the setting of
// the read buffer size and the time between pollings for testing.
func newTailer(readSeeker io.ReadSeeker, writer io.Writer, lines int, filter TailerFilterFunc,
	bufferSize int, polltime time.Duration) *Tailer {
	t := &Tailer{
		readSeeker: readSeeker,
		reader:     bufio.NewReaderSize(readSeeker, bufferSize),
		writer:     bufio.NewWriter(writer),
		lines:      lines,
		filter:     filter,
		bufferSize: bufferSize,
		polltime:   polltime,
	}
	go func() {
		defer t.tomb.Done()
		t.tomb.Kill(t.loop())
	}()
	return t
}

// Stop tells the tailer to stop working.
func (t *Tailer) Stop() error {
	t.tomb.Kill(nil)
	return t.tomb.Wait()
}

// Wait waits until the tailer is stopped due to command
// or an error. In case of an error it returns the reason.
func (t *Tailer) Wait() error {
	return t.tomb.Wait()
}

// Dead returns the channel that can be used to wait until
// the tailer is stopped.
func (t *Tailer) Dead() <-chan struct{} {
	return t.tomb.Dead()
}

// Err returns a possible error.
func (t *Tailer) Err() error {
	return t.tomb.Err()
}

// loop writes the last lines based on the buffer size to the
// writer and then polls for more data to write it to the
// writer too.
func (t *Tailer) loop() error {
	// Position the readSeeker.
	if err := t.seekLastLines(); err != nil {
		return err
	}
	// Start polling.
	// TODO(mue) 2013-12-06
	// Handling of read-seeker/files being truncated during
	// tailing is currently missing!
	timer := time.NewTimer(0)
	for {
		select {
		case <-t.tomb.Dying():
			return nil
		case <-timer.C:
			for {
				line, readErr := t.readLine()
				_, writeErr := t.writer.Write(line)
				if writeErr != nil {
					return writeErr
				}
				if readErr != nil {
					if readErr != io.EOF {
						return readErr
					}
					break
				}
			}
			if writeErr := t.writer.Flush(); writeErr != nil {
				return writeErr
			}
			timer.Reset(t.polltime)
		}
	}
}

// seekLastLines sets the read position of the ReadSeeker to the
// wanted number of filtered lines before the end.
func (t *Tailer) seekLastLines() error {
	offset, err := t.readSeeker.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}
	seekPos := int64(0)
	found := 0
	buffer := make([]byte, t.bufferSize)
SeekLoop:
	for offset > 0 {
		// buffer contains the data left over from the
		// previous iteration.
		space := cap(buffer) - len(buffer)
		if space < t.bufferSize {
			// Grow buffer.
			newBuffer := make([]byte, len(buffer), cap(buffer)*2)
			copy(newBuffer, buffer)
			buffer = newBuffer
			space = cap(buffer) - len(buffer)
		}
		if int64(space) > offset {
			// Use exactly the right amount of space if there's
			// only a small amount remaining.
			space = int(offset)
		}
		// Copy data remaining from last time to the end of the buffer,
		// so we can read into the right place.
		copy(buffer[space:cap(buffer)], buffer)
		buffer = buffer[0 : len(buffer)+space]
		offset -= int64(space)
		_, err := t.readSeeker.Seek(offset, os.SEEK_SET)
		if err != nil {
			return err
		}
		_, err = io.ReadFull(t.readSeeker, buffer[0:space])
		if err != nil {
			return err
		}
		// Find the end of the last line in the buffer.
		// This will discard any unterminated line at the end
		// of the file.
		end := bytes.LastIndex(buffer, delimiters)
		if end == -1 {
			// No end of line found - discard incomplete
			// line and continue looking. If this happens
			// at the beginning of the file, we don't care
			// because we're going to stop anyway.
			buffer = buffer[:0]
			continue
		}
		end++
		for {
			start := bytes.LastIndex(buffer[0:end-1], delimiters)
			if start == -1 && offset >= 0 {
				break
			}
			start++
			if t.isValid(buffer[start:end]) {
				found++
				if found >= t.lines {
					seekPos = offset + int64(start)
					break SeekLoop
				}
			}
			end = start
		}
		// Leave the last line in buffer, as we don't know whether
		// it's complete or not.
		buffer = buffer[0:end]
	}
	// Final positioning.
	t.readSeeker.Seek(seekPos, os.SEEK_SET)
	return nil
}

// readLine reads the next valid line from the reader, even if it is
// larger than the reader buffer.
func (t *Tailer) readLine() ([]byte, error) {
	for {
		slice, err := t.reader.ReadSlice(delimiter)
		if err == nil {
			if t.isValid(slice) {
				return slice, nil
			}
			continue
		}
		line := append([]byte(nil), slice...)
		for err == bufio.ErrBufferFull {
			slice, err = t.reader.ReadSlice(delimiter)
			line = append(line, slice...)
		}
		switch err {
		case nil:
			if t.isValid(line) {
				return line, nil
			}
		case io.EOF:
			// EOF without delimiter, step back.
			t.readSeeker.Seek(-int64(len(line)), os.SEEK_CUR)
			return nil, err
		default:
			return nil, err
		}
	}
}

// isValid checks if the passed line is valid by checking if the
// line has content, the filter function is nil or it returns true.
func (t *Tailer) isValid(line []byte) bool {
	if t.filter == nil {
		return true
	}
	return t.filter(line)
}
