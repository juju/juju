// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tailer_test

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils/tailer"
)

func Test(t *testing.T) {
	gc.TestingT(t)
}

type tailerSuite struct{}

var _ = gc.Suite(tailerSuite{})

var tests = []struct {
	description           string
	data                  []string
	initialLinesWritten   int
	initialLinesRequested int
	bufferSize            int
	filter                tailer.TailerFilterFunc
	injector              func(*tailer.Tailer, *readSeeker) func([]string)
	initialCollectedData  []string
	appendedCollectedData []string
	err                   string
}{
	{
		description:           "lines are longer than buffer size",
		data:                  data[26:29],
		initialLinesWritten:   1,
		initialLinesRequested: 1,
		bufferSize:            5,
		initialCollectedData:  data[26:27],
		appendedCollectedData: data[27:28],
	}, {
		description:           "lines are longer than buffer size, missing termination of last line",
		data:                  data[26:30],
		initialLinesWritten:   1,
		initialLinesRequested: 1,
		bufferSize:            5,
		initialCollectedData:  data[26:27],
		appendedCollectedData: data[27:28],
	}, {
		description:           "lines are longer than buffer size, last line is terminated later",
		data:                  data[26:30],
		initialLinesWritten:   1,
		initialLinesRequested: 1,
		bufferSize:            5,
		initialCollectedData:  data[26:27],
		appendedCollectedData: []string{data[27], data[28] + data[29]},
	}, {
		description:           "missing termination of last line",
		data:                  data[26:29],
		initialLinesWritten:   1,
		initialLinesRequested: 1,
		initialCollectedData:  data[26:27],
		appendedCollectedData: data[27:28],
	}, {
		description:           "last line is terminated later",
		data:                  data[26:30],
		initialLinesWritten:   1,
		initialLinesRequested: 1,
		initialCollectedData:  data[26:27],
		appendedCollectedData: []string{data[27], data[28] + data[29]},
	}, {
		description:           "more lines already written than initially requested",
		data:                  data[:26],
		initialLinesWritten:   5,
		initialLinesRequested: 3,
		initialCollectedData:  data[2:5],
		appendedCollectedData: data[5:26],
	}, {
		description:           "less lines already written than initially requested",
		data:                  data[:26],
		initialLinesWritten:   3,
		initialLinesRequested: 5,
		initialCollectedData:  data[0:3],
		appendedCollectedData: data[3:26],
	}, {
		description:           "lines are longer than buffer size, more lines already written than initially requested",
		data:                  data[:26],
		initialLinesWritten:   5,
		initialLinesRequested: 3,
		bufferSize:            5,
		initialCollectedData:  data[2:5],
		appendedCollectedData: data[5:26],
	}, {
		description:           "lines are longer than buffer size, less lines already written than initially requested",
		data:                  data[:26],
		initialLinesWritten:   3,
		initialLinesRequested: 5,
		bufferSize:            5,
		initialCollectedData:  data[0:3],
		appendedCollectedData: data[3:26],
	}, {
		description:           "filter lines which contain the char 'e'",
		data:                  data[:26],
		initialLinesWritten:   10,
		initialLinesRequested: 3,
		filter: func(line []byte) bool {
			return bytes.Contains(line, []byte{'e'})
		},
		initialCollectedData:  []string{data[4], data[7], data[9]},
		appendedCollectedData: []string{data[12], data[13], data[16], data[17], data[18], data[22], data[24]},
	}, {
		description:           "stop tailing after 10 collected lines",
		data:                  data[:26],
		initialLinesWritten:   5,
		initialLinesRequested: 3,
		injector: func(t *tailer.Tailer, rs *readSeeker) func([]string) {
			return func(lines []string) {
				if len(lines) == 10 {
					t.Stop()
				}
			}
		},
		initialCollectedData:  data[2:5],
		appendedCollectedData: data[5:26],
	}, {
		description:           "generate an error after 10 collected lines",
		data:                  data[:26],
		initialLinesWritten:   5,
		initialLinesRequested: 3,
		injector: func(t *tailer.Tailer, rs *readSeeker) func([]string) {
			return func(lines []string) {
				if len(lines) == 10 {
					rs.setError(fmt.Errorf("ouch after 10 lines"))
				}
			}
		},
		initialCollectedData:  data[2:5],
		appendedCollectedData: data[5:26],
		err: "ouch after 10 lines",
	}, {
		description:           "more lines already written than initially requested, some empty, unfiltered",
		data:                  data[30:40],
		initialLinesWritten:   3,
		initialLinesRequested: 2,
		initialCollectedData:  data[31:33],
		appendedCollectedData: data[33:40],
	}, {
		description:           "more lines already written than initially requested, some empty, those filtered",
		data:                  data[30:40],
		initialLinesWritten:   3,
		initialLinesRequested: 2,
		filter: func(line []byte) bool {
			return len(bytes.TrimSpace(line)) > 0
		},
		initialCollectedData:  data[30:32],
		appendedCollectedData: []string{data[34], data[35], data[38], data[39]},
	},
}

func (tailerSuite) TestTailer(c *gc.C) {
	for i, test := range tests {
		c.Logf("Test #%d) %s", i, test.description)
		bufferSize := test.bufferSize
		if bufferSize == 0 {
			// Default value.
			bufferSize = 4096
		}
		reader, writer := io.Pipe()
		sigc := make(chan struct{}, 1)
		rs := startReadSeeker(c, test.data, test.initialLinesWritten, sigc)

		t := tailer.NewTestTailer(rs, writer, test.initialLinesRequested, test.filter, bufferSize, 2*time.Millisecond)

		// Collect initial data.
		assertCollected(c, reader, test.initialCollectedData, nil)

		sigc <- struct{}{}

		// Collect remaining data, possibly with injection to stop
		// earlier or generate an error.
		var injection func([]string)
		if test.injector != nil {
			injection = test.injector(t, rs)
		}

		assertCollected(c, reader, test.appendedCollectedData, injection)

		if test.err == "" {
			c.Assert(t.Stop(), gc.IsNil)
		} else {
			c.Assert(t.Err(), gc.ErrorMatches, test.err)
		}
	}
}

// assertCollected reads lines out of the reader used by the Tailer
// to write the collected data in. It compares if those are the one passed
// with compare until the timeout. If this time is reached earlier the
// assertion fails. The injection function allows to interrupt the processing
// with a function generating an error or a regular stopping during
// the tailing. As in this case the lines to compare will no be reached
// the timeout will not be interpreted as failure.
func assertCollected(c *gc.C, reader io.Reader, compare []string, injection func([]string)) {
	buffer := bufio.NewReader(reader)
	timeout := time.Now().Add(250 * time.Millisecond)
	lines := []string{}
	for {
		line, err := buffer.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, line)
			if injection != nil {
				injection(lines)
			}
			if len(lines) == len(compare) {
				for i := 0; i < len(lines); i++ {
					c.Assert(lines[i], gc.Equals, compare[i])
				}
				return
			}
		}
		if err == io.EOF {
			if time.Now().Sub(timeout) > 0 {
				if injection == nil {
					c.Fatalf("timeout during tailer collection")
				}
				return
			}
			time.Sleep(time.Millisecond)
			continue
		}
		c.Assert(err, gc.IsNil)
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

var data = []string{
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
	"abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyz\n",
	"0123456789012345678901234567890123456789012345678901\n",
	"the quick brown fox ",
	"jumps over the lazy dog\n",
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
}

var tailerData = data
