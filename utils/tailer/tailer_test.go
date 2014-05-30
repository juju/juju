// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tailer_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/juju/testing"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/tailer"
)

type tailerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&tailerSuite{})

var alphabetData = []string{
	"alpha alpha\n",
	"bravo bravo\n",
	"charlie charlie\n",
	"delta delta\n",
	"echo echo\n",
	"foxtrott foxtrott\n",
	"golf golf\n",
	"hotel hotel\n",
	"india india\n",
	"juliet juliet\n",
	"kilo kilo\n",
	"lima lima\n",
	"mike mike\n",
	"november november\n",
	"oscar oscar\n",
	"papa papa\n",
	"quebec quebec\n",
	"romeo romeo\n",
	"sierra sierra\n",
	"tango tango\n",
	"uniform uniform\n",
	"victor victor\n",
	"whiskey whiskey\n",
	"x-ray x-ray\n",
	"yankee yankee\n",
	"zulu zulu\n",
}

var tests = []struct {
	description           string
	data                  []string
	initialLinesWritten   int
	initialLinesRequested uint
	bufferSize            int
	filter                tailer.TailerFilterFunc
	injector              func(*tailer.Tailer, *readSeeker) func([]string)
	initialCollectedData  []string
	appendedCollectedData []string
	fromStart             bool
	err                   string
}{{
	description: "lines are longer than buffer size",
	data: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
		"0123456789012345678901234567890123456789012345678901\n",
	},
	initialLinesWritten:   1,
	initialLinesRequested: 1,
	bufferSize:            5,
	initialCollectedData: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
	},
	appendedCollectedData: []string{
		"0123456789012345678901234567890123456789012345678901\n",
	},
}, {
	description: "lines are longer than buffer size, missing termination of last line",
	data: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
		"0123456789012345678901234567890123456789012345678901\n",
		"the quick brown fox ",
	},
	initialLinesWritten:   1,
	initialLinesRequested: 1,
	bufferSize:            5,
	initialCollectedData: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
	},
	appendedCollectedData: []string{
		"0123456789012345678901234567890123456789012345678901\n",
	},
}, {
	description: "lines are longer than buffer size, last line is terminated later",
	data: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
		"0123456789012345678901234567890123456789012345678901\n",
		"the quick brown fox ",
		"jumps over the lazy dog\n",
	},
	initialLinesWritten:   1,
	initialLinesRequested: 1,
	bufferSize:            5,
	initialCollectedData: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
	},
	appendedCollectedData: []string{
		"0123456789012345678901234567890123456789012345678901\n",
		"the quick brown fox jumps over the lazy dog\n",
	},
}, {
	description: "missing termination of last line",
	data: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
		"0123456789012345678901234567890123456789012345678901\n",
		"the quick brown fox ",
	},
	initialLinesWritten:   1,
	initialLinesRequested: 1,
	initialCollectedData: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
	},
	appendedCollectedData: []string{
		"0123456789012345678901234567890123456789012345678901\n",
	},
}, {
	description: "last line is terminated later",
	data: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
		"0123456789012345678901234567890123456789012345678901\n",
		"the quick brown fox ",
		"jumps over the lazy dog\n",
	},
	initialLinesWritten:   1,
	initialLinesRequested: 1,
	initialCollectedData: []string{
		"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
	},
	appendedCollectedData: []string{
		"0123456789012345678901234567890123456789012345678901\n",
		"the quick brown fox jumps over the lazy dog\n",
	},
}, {
	description:           "more lines already written than initially requested",
	data:                  alphabetData,
	initialLinesWritten:   5,
	initialLinesRequested: 3,
	initialCollectedData: []string{
		"charlie charlie\n",
		"delta delta\n",
		"echo echo\n",
	},
	appendedCollectedData: alphabetData[5:],
}, {
	description:           "less lines already written than initially requested",
	data:                  alphabetData,
	initialLinesWritten:   3,
	initialLinesRequested: 5,
	initialCollectedData: []string{
		"alpha alpha\n",
		"bravo bravo\n",
		"charlie charlie\n",
	},
	appendedCollectedData: alphabetData[3:],
}, {
	description:           "lines are longer than buffer size, more lines already written than initially requested",
	data:                  alphabetData,
	initialLinesWritten:   5,
	initialLinesRequested: 3,
	bufferSize:            5,
	initialCollectedData: []string{
		"charlie charlie\n",
		"delta delta\n",
		"echo echo\n",
	},
	appendedCollectedData: alphabetData[5:],
}, {
	description:           "ignore current lines",
	data:                  alphabetData,
	initialLinesWritten:   5,
	bufferSize:            5,
	appendedCollectedData: alphabetData[5:],
}, {
	description:           "start from the start",
	data:                  alphabetData,
	initialLinesWritten:   5,
	bufferSize:            5,
	appendedCollectedData: alphabetData,
	fromStart:             true,
}, {
	description:           "lines are longer than buffer size, less lines already written than initially requested",
	data:                  alphabetData,
	initialLinesWritten:   3,
	initialLinesRequested: 5,
	bufferSize:            5,
	initialCollectedData: []string{
		"alpha alpha\n",
		"bravo bravo\n",
		"charlie charlie\n",
	},
	appendedCollectedData: alphabetData[3:],
}, {
	description:           "filter lines which contain the char 'e'",
	data:                  alphabetData,
	initialLinesWritten:   10,
	initialLinesRequested: 3,
	filter: func(line []byte) bool {
		return bytes.Contains(line, []byte{'e'})
	},
	initialCollectedData: []string{
		"echo echo\n",
		"hotel hotel\n",
		"juliet juliet\n",
	},
	appendedCollectedData: []string{
		"mike mike\n",
		"november november\n",
		"quebec quebec\n",
		"romeo romeo\n",
		"sierra sierra\n",
		"whiskey whiskey\n",
		"yankee yankee\n",
	},
}, {
	description:           "stop tailing after 10 collected lines",
	data:                  alphabetData,
	initialLinesWritten:   5,
	initialLinesRequested: 3,
	injector: func(t *tailer.Tailer, rs *readSeeker) func([]string) {
		return func(lines []string) {
			if len(lines) == 10 {
				t.Stop()
			}
		}
	},
	initialCollectedData: []string{
		"charlie charlie\n",
		"delta delta\n",
		"echo echo\n",
	},
	appendedCollectedData: alphabetData[5:],
}, {
	description:           "generate an error after 10 collected lines",
	data:                  alphabetData,
	initialLinesWritten:   5,
	initialLinesRequested: 3,
	injector: func(t *tailer.Tailer, rs *readSeeker) func([]string) {
		return func(lines []string) {
			if len(lines) == 10 {
				rs.setError(fmt.Errorf("ouch after 10 lines"))
			}
		}
	},
	initialCollectedData: []string{
		"charlie charlie\n",
		"delta delta\n",
		"echo echo\n",
	},
	appendedCollectedData: alphabetData[5:],
	err: "ouch after 10 lines",
}, {
	description: "more lines already written than initially requested, some empty, unfiltered",
	data: []string{
		"one one\n",
		"two two\n",
		"\n",
		"\n",
		"three three\n",
		"four four\n",
		"\n",
		"\n",
		"five five\n",
		"six six\n",
	},
	initialLinesWritten:   3,
	initialLinesRequested: 2,
	initialCollectedData: []string{
		"two two\n",
		"\n",
	},
	appendedCollectedData: []string{
		"\n",
		"three three\n",
		"four four\n",
		"\n",
		"\n",
		"five five\n",
		"six six\n",
	},
}, {
	description: "more lines already written than initially requested, some empty, those filtered",
	data: []string{
		"one one\n",
		"two two\n",
		"\n",
		"\n",
		"three three\n",
		"four four\n",
		"\n",
		"\n",
		"five five\n",
		"six six\n",
	},
	initialLinesWritten:   3,
	initialLinesRequested: 2,
	filter: func(line []byte) bool {
		return len(bytes.TrimSpace(line)) > 0
	},
	initialCollectedData: []string{
		"one one\n",
		"two two\n",
	},
	appendedCollectedData: []string{
		"three three\n",
		"four four\n",
		"five five\n",
		"six six\n",
	},
}}

func (s *tailerSuite) TestTailer(c *gc.C) {
	for i, test := range tests {
		c.Logf("Test #%d) %s", i, test.description)
		bufferSize := test.bufferSize
		if bufferSize == 0 {
			// Default value.
			bufferSize = 4096
		}
		s.PatchValue(tailer.BufferSize, bufferSize)
		reader, writer := io.Pipe()
		sigc := make(chan struct{}, 1)
		rs := startReadSeeker(c, test.data, test.initialLinesWritten, sigc)
		if !test.fromStart {
			err := tailer.SeekLastLines(rs, test.initialLinesRequested, test.filter)
			c.Assert(err, gc.IsNil)
		}
		tailer := tailer.NewTestTailer(rs, writer, test.filter, 2*time.Millisecond)
		linec := startReading(c, tailer, reader, writer)

		// Collect initial data.
		assertCollected(c, linec, test.initialCollectedData, nil)

		sigc <- struct{}{}

		// Collect remaining data, possibly with injection to stop
		// earlier or generate an error.
		var injection func([]string)
		if test.injector != nil {
			injection = test.injector(tailer, rs)
		}

		assertCollected(c, linec, test.appendedCollectedData, injection)

		if test.err == "" {
			c.Assert(tailer.Stop(), gc.IsNil)
		} else {
			c.Assert(tailer.Err(), gc.ErrorMatches, test.err)
		}
	}
}

// startReading starts a goroutine receiving the lines out of the reader
// in the background and passing them to a created string channel. This
// will used in the assertions.
func startReading(c *gc.C, tailer *tailer.Tailer, reader *io.PipeReader, writer *io.PipeWriter) chan string {
	linec := make(chan string)
	// Start goroutine for reading.
	go func() {
		defer close(linec)
		reader := bufio.NewReader(reader)
		for {
			line, err := reader.ReadString('\n')
			switch err {
			case nil:
				linec <- line
			case io.EOF:
				return
			default:
				c.Fail()
			}
		}
	}()
	// Close writer when tailer is stopped or has an error. Tailer using
	// components can do it the same way.
	go func() {
		tailer.Wait()
		writer.Close()
	}()
	return linec
}

// assertCollected reads lines from the string channel linec. It compares if
// those are the one passed with compare until a timeout. If the timeout is
// reached earlier than all lines are collected the assertion fails. The
// injection function allows to interrupt the processing with a function
// generating an error or a regular stopping during the tailing. In case the
// linec is closed due to stopping or an error only the values so far care
// compared. Checking the reason for termination is done in the test.
func assertCollected(c *gc.C, linec chan string, compare []string, injection func([]string)) {
	if len(compare) == 0 {
		return
	}
	timeout := time.After(10 * time.Second)
	lines := []string{}
	for {
		select {
		case line, ok := <-linec:
			if ok {
				lines = append(lines, line)
				if injection != nil {
					injection(lines)
				}
				if len(lines) == len(compare) {
					// All data received.
					c.Assert(lines, gc.DeepEquals, compare)
					return
				}
			} else {
				// linec closed after stopping or error.
				c.Assert(lines, gc.DeepEquals, compare[:len(lines)])
				return
			}
		case <-timeout:
			if injection == nil {
				c.Fatalf("timeout during tailer collection")
			}
			return
		}
	}
}

// startReadSeeker returns a ReadSeeker for the Tailer. It simulates
// reading and seeking inside a file and also simulating an error.
// The goroutine waits for a signal that it can start writing the
// appended lines.
func startReadSeeker(c *gc.C, data []string, initialLeg int, sigc chan struct{}) *readSeeker {
	// Write initial lines into the buffer.
	var rs readSeeker
	var i int
	for i = 0; i < initialLeg; i++ {
		rs.write(data[i])
	}

	go func() {
		<-sigc

		for ; i < len(data); i++ {
			time.Sleep(5 * time.Millisecond)
			rs.write(data[i])
		}
	}()
	return &rs
}

type readSeeker struct {
	mux    sync.Mutex
	buffer []byte
	pos    int
	err    error
}

func (r *readSeeker) write(s string) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.buffer = append(r.buffer, []byte(s)...)
}

func (r *readSeeker) setError(err error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.err = err
}

func (r *readSeeker) Read(p []byte) (n int, err error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	if r.err != nil {
		return 0, r.err
	}
	if r.pos >= len(r.buffer) {
		return 0, io.EOF
	}
	n = copy(p, r.buffer[r.pos:])
	r.pos += n
	return n, nil
}

func (r *readSeeker) Seek(offset int64, whence int) (ret int64, err error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	var newPos int64
	switch whence {
	case 0:
		newPos = offset
	case 1:
		newPos = int64(r.pos) + offset
	case 2:
		newPos = int64(len(r.buffer)) + offset
	default:
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	if newPos < 0 {
		return 0, fmt.Errorf("negative position: %d", newPos)
	}
	if newPos >= 1<<31 {
		return 0, fmt.Errorf("position out of range: %d", newPos)
	}
	r.pos = int(newPos)
	return newPos, nil
}
