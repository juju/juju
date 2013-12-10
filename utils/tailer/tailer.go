// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tailer

import (
	"bufio"
	"io"
	"os"
	"time"

	"launchpad.net/tomb"
)

const (
	bufsize   = 4096
	polltime  = time.Second
	delimiter = '\n'
)

// TailerFilterFunc decides if a line shall be tailed (func is nil or
// returns true) of shall be omitted (func returns false).
type TailerFilterFunc func(line []byte) bool

// Tailer reads an input line by line an tails them into the passed Writer.
// The lines have to be terminated with a newline.
type Tailer struct {
	tomb       tomb.Tomb
	readSeeker io.ReadSeeker
	reader     *bufio.Reader
	writer     *bufio.Writer
	lines      int
	filter     TailerFilterFunc
	bufsize    int
	polltime   time.Duration
}

// NewStandardTailer starts a Tailer which reads strings from the passed
// ReadSeeker line by line. If a filter function is specified the read
// lines are filtered. The matching lines are written to the passed
// Writer. The reading beginns the specified number of matching lines
// from the end.
func NewStandardTailer(readSeeker io.ReadSeeker, writer io.Writer, lines int, filter TailerFilterFunc) *Tailer {
	return NewTailer(readSeeker, writer, lines, filter, bufsize, polltime)
}

// NewTailer starts a Tailer like NewStandardTailer but allows some tuning
// by defining the size of the read buffer and the time between pollings.
func NewTailer(readSeeker io.ReadSeeker, writer io.Writer, lines int, filter TailerFilterFunc,
	bufsize int, polltime time.Duration) *Tailer {
	t := &Tailer{
		readSeeker: readSeeker,
		reader:     bufio.NewReaderSize(readSeeker, bufsize),
		writer:     bufio.NewWriter(writer),
		lines:      lines,
		filter:     filter,
		bufsize:    bufsize,
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

// seekLastLines sets the read position of the ReadSeeker the
// wanted number of filtered lines before the end.
func (t *Tailer) seekLastLines() error {
	// Start at the end.
	offset, err := t.readSeeker.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}
	seekPos := int64(0)
	readBuffer := make([]byte, t.bufsize)
	lineBuffer := []byte(nil)
	found := 0
	beginp := -1
	endp := -1
	first := true
	// Seek backwards.
SeekLoop:
	for {
		// Read new block and prepare line buffer for scanning.
		newOffset := offset - int64(len(readBuffer))
		if newOffset < 0 {
			newOffset = 0
		}
		_, err := t.readSeeker.Seek(newOffset, os.SEEK_SET)
		if err != nil {
			return err
		}
		n := int(offset - newOffset)
		offset = newOffset
		n, err = t.readSeeker.Read(readBuffer[0:n])
		if err != nil {
			return err
		}
		newBuffer := make([]byte, n+len(lineBuffer))
		copy(newBuffer, readBuffer[0:n])
		copy(newBuffer[n:], lineBuffer)
		lineBuffer = newBuffer
		endp += n
		beginp += n
		// Scan line buffer for contained lines. If the last line of
		// the first read block is not delimited it will be skipped.
		// So the following readLine() of the main loop can read
		// and check it fully.
	ScanLoop:
		for {
			// First try to find the terminating delimiter.
			if first {
				for endp >= 0 && lineBuffer[endp] != delimiter {
					endp--
				}
				if endp <= 0 {
					// No ending or exact in the beginning.
					// So read next block.
					break ScanLoop
				}
				first = false
				beginp = endp - 1
			}
			// Now the delimiter of the preceding line.
			for beginp >= 0 && lineBuffer[beginp] != delimiter {
				beginp--
			}
			if beginp < 0 {
				// No next delimiter aka beginnig of this
				// line. So read next block.
				break ScanLoop
			}
			// Found a line inside the buffer. Check it.
			if t.isValid(lineBuffer[beginp+1 : endp+1]) {
				found++
				if found == t.lines {
					seekPos = offset + int64(beginp+1)
					break SeekLoop
				}
			}
			// Not valid or not enough, prepare next round.
			endp = beginp
			beginp = endp - 1
			lineBuffer = lineBuffer[:endp+1]
		}
		if offset == 0 {
			// Reached beginnig of data.
			break SeekLoop
		}
	}
	// Final positioning.
	t.readSeeker.Seek(seekPos, os.SEEK_SET)
	return nil
}

// readLine reads the next valid line from the reader, even if it is
// larger than the reader buffer.
func (t *Tailer) readLine() ([]byte, error) {
	line := []byte(nil)
	for {
		buffer, err := t.reader.ReadBytes(delimiter)
		line = append(line, buffer...)
		switch err {
		case nil:
			// Found next line, return if valid, else drop.
			if t.isValid(line) {
				return line, nil
			}
			line = nil
		case bufio.ErrBufferFull:
			// More to read.
			continue
		case io.EOF:
			// EOF, do we have a terminated line?
			if len(line) == 0 || line[len(line)-1] == delimiter {
				if t.isValid(line) {
					return line, err
				}
				line = nil
			}
			// Step back.
			offset := int64(-len(line))
			t.readSeeker.Seek(offset, os.SEEK_END)
			return nil, err
		default:
			// Other error.
			return line, err
		}
	}
}

// isValid checks if the passed line is valid by checking if the
// line has content, the filter function is nil or it returns true.
func (t *Tailer) isValid(line []byte) bool {
	if len(line) == 0 {
		return false
	}
	if t.filter == nil {
		return true
	}
	return t.filter(line)
}
