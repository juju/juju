// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils

import (
	"bufio"
	"io"
	"os"
	"time"

	"launchpad.net/tomb"
)

// TailerFilterFunc decides if a line shall be tailed (func is nil or
// returns true) of shall be omitted (func returns false).
type TailerFilterFunc func(line string) bool

// Tailer reads an input line by line an tails them into the passed Writer.
type Tailer struct {
	tomb       tomb.Tomb
	readSeeker io.ReadSeeker
	lines      int
	filter     TailerFilterFunc
	polltime   time.Duration
	writer     io.Writer
}

// StartFileTailer opens the file and starts the tailer.
func StartFileTailer(filename string, lines int, filter TailerFilterFunc,
	polltime time.Duration, writer io.Writer) (*Tailer, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	return StartTailer(file, lines, filter, polltime, writer), nil
}

// StartTailer starts the tailer for the passed ReadSeeker.
func StartTailer(readSeeker io.ReadSeeker, lines int, filter TailerFilterFunc,
	polltime time.Duration, writer io.Writer) *Tailer {
	t := &Tailer{
		readSeeker: readSeeker,
		lines:      lines,
		filter:     filter,
		polltime:   polltime,
		writer:     writer,
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
	// Do the initial reading into the buffer.
	reader := bufio.NewReader(t.readSeeker)
	writer := bufio.NewWriter(t.writer)
	buffer := make([]string, t.lines)
	bufPointer := 0
	bufCount := 0
	for {
		line, err := reader.ReadString('\n')
		if len(line) > 0 {
			if t.filter == nil || t.filter(line) {
				buffer[bufPointer] = line
				bufPointer = (bufPointer + 1) % t.lines
				if bufCount < t.lines {
					bufCount++
				}
			}
		}
		if err != nil {
			if err != io.EOF {
				return err
			}
			break
		}
	}
	// Write so far collected lines.
	start := (bufPointer - bufCount + t.lines) % t.lines
	bufPointer = start
	for bufCount > 0 {
		writer.WriteString(buffer[bufPointer])
		bufPointer = (bufPointer + 1) % t.lines
		bufCount--
	}
	// Poll the file for new appended data.
	timer := time.NewTimer(t.polltime)
	for {
		select {
		case <-t.tomb.Dying():
			return nil
		case <-timer.C:
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					if t.filter == nil || t.filter(line) {
						writer.WriteString(line)
					}
				}
				if err != nil {
					if err != io.EOF {
						return err
					}
					break
				}
			}
			writer.Flush()
			t.readSeeker.Seek(0, os.SEEK_END)
			timer.Reset(t.polltime)
		}
	}
}
