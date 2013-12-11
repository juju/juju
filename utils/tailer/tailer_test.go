// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tailer_test

import (
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

func (tailerSuite) TestMissingTermination(c *gc.C) {
	// Last line is not terminated.
	data := []string{"One\n", "Two\n", "Three\n", "Four"}
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, data, 2, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 2, nil, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, data[:2], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, []string{data[2]}, nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLaggedTermination(c *gc.C) {
	// Last line is terminated later.
	data := []string{"One\n", "Two\n", "Three\n", "Four", "teen\n"}
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, data, 2, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 2, nil, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, data[:2], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, []string{data[2], data[3] + data[4]}, nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLargeLines(c *gc.C) {
	// Single lines are larger than the buffer.
	data := []string{"abcdefghijklmnopqrstuvwxyz\n", "01234567890123456789012345\n"}
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, data, 1, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 1, nil, 5, 2*time.Millisecond)

	assertCollected(c, buffer, data[:1], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, data[1:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLargeLinesMissingTermination(c *gc.C) {
	// Single lines are larger than the buffer, last line not terminated.
	data := []string{"abcdefghijklmnopqrstuvwxyz\n", "01234567890123456789012345\n",
		"the quick brown fox"}
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, data, 1, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 1, nil, 5, 2*time.Millisecond)

	assertCollected(c, buffer, data[:1], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, []string{data[1]}, nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLargeLinesLaggedTermination(c *gc.C) {
	// Single lines are larger than the buffer, last line is terminated later.
	data := []string{"abcdefghijklmnopqrstuvwxyz\n", "01234567890123456789012345\n",
		"the quick brown fox", " jumps over the lazy dog\n"}
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, data, 1, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 1, nil, 5, 2*time.Millisecond)

	assertCollected(c, buffer, data[:1], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, []string{data[1], data[2] + data[3]}, nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestMoreThanTailed(c *gc.C) {
	// Tailer initially asks for less lines than exist.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 5, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 3, nil, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[2:5], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[5:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLessThanTailed(c *gc.C) {
	// Tailer initially asks for more lines than exist so far.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 3, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 5, nil, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[0:3], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[3:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestMoreThanTailedSmallBuffer(c *gc.C) {
	// Tailer initially asks for less lines than exist. Buffer
	// is smaller than the data, so multiple reads are needed.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 5, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 3, nil, 16, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[2:5], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[5:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLessThanTailedSmallBuffer(c *gc.C) {
	// Tailer initially asks for more lines than exist so far.
	// Buffer is smaller than the data, so multiple reads
	// are needed.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 3, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 5, nil, 16, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[0:3], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[3:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestFiltered(c *gc.C) {
	// Only return lines containing an 'a'.
	buffer := bytes.NewBuffer(nil)
	filter := func(line []byte) bool {
		return bytes.Contains(line, []byte{'e'})
	}
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 10, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 3, filter, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, []string{"Delta\n", "Hotel\n", "Juliet\n"}, nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, []string{"Mike\n", "November\n", "Quebec\n",
		"Romeo\n", "Sierra\n", "Whiskey\n", "Yankee\n"}, nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestStop(c *gc.C) {
	// Stop after collecting 10 lines.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 5, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 3, nil, 4096, 2*time.Millisecond)
	stopper := func(lines []string) {
		if len(lines) == 10 {
			c.Assert(t.Stop(), gc.IsNil)
		}
	}

	assertCollected(c, buffer, tailerData[2:5], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[5:], stopper)

	c.Assert(t.Err(), gc.IsNil)
}

func (tailerSuite) TestError(c *gc.C) {
	// Generate a read error after collecting 10 lines.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, tailerData, 5, sigc)

	<-sigc

	t := tailer.NewTestTailer(rs, buffer, 3, nil, 4096, 2*time.Millisecond)
	disturber := func(lines []string) {
		if len(lines) == 10 {
			rs.setError(fmt.Errorf("ouch after 10 lines"))
		}
	}

	assertCollected(c, buffer, tailerData[2:5], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[5:], disturber)

	c.Assert(t.Err(), gc.ErrorMatches, "ouch after 10 lines")
}

const timeout = 250 * time.Millisecond

// assertCollected reads lines out of the buffer where there have been
// written by the Tailer. It compares of those are the one passed with
// compare during the timespan defined as timeout. If this time is
// reached earlier the assertion fails. The addon function allows to
// inject a function generating an error or a regular stopping during
// the tailing. As in this case the lines to compare will no be reached
// the timeout will not be interpreted as failure.
func assertCollected(c *gc.C, buffer *bytes.Buffer, compare []string, addon func([]string)) {
	start := time.Now()
	lines := []string{}
	for {
		line, err := buffer.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, line)
			if addon != nil {
				addon(lines)
			}
			if len(lines) == len(compare) {
				for i := 0; i < len(lines); i++ {
					c.Assert(lines[i], gc.Equals, compare[i])
				}
				return
			}
		}
		if err == io.EOF {
			if time.Now().Sub(start) > timeout {
				if addon == nil {
					c.Fatalf("timeout during collecting")
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
// The signal channel tells the test that the initial data has been
// written and waits that it can start writing the appended lines.
func startReadSeeker(c *gc.C, data []string, initialLeg int, sigc chan struct{}) *readSeeker {
	// Write initial lines into the buffer.
	var rs readSeeker
	var i int
	for i = 0; i < initialLeg; i++ {
		rs.write(data[i])
	}

	sigc <- struct{}{}

	// Continue with the rest in the background.
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

var tailerData = []string{
	"Alpha\n",
	"Bravo\n",
	"Charlie\n",
	"Delta\n",
	"Echo\n",
	"Foxtrott\n",
	"Golf\n",
	"Hotel\n",
	"India\n",
	"Juliet\n",
	"Kilo\n",
	"Lima\n",
	"Mike\n",
	"November\n",
	"Oscar\n",
	"Papa\n",
	"Quebec\n",
	"Romeo\n",
	"Sierra\n",
	"Tango\n",
	"Uniform\n",
	"Victor\n",
	"Whiskey\n",
	"X-ray\n",
	"Yankee\n",
	"Zulu\n",
}
