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

func (tailerSuite) TestMoreThanTailed(c *gc.C) {
	// Tails asks for less lines than exist.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 3, nil, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[2:5], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[5:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLessThanTailed(c *gc.C) {
	// Tail asks initially for more lines than exist so far.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 3, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 5, nil, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[0:3], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[3:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestMoreThanTailedSmallBuffer(c *gc.C) {
	// Tails asks for less lines than exist. Buffer is
	// smaller than the data, so multiple reads are needed.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 3, nil, 16, 2*time.Millisecond)

	assertCollected(c, buffer, tailerData[2:5], nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, tailerData[5:], nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLessThanTailedSmallBuffer(c *gc.C) {
	// Tail asks initially for more lines than exist so far.
	// Buffer is smaller than the data, so multiple reads
	// are needed.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 3, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 5, nil, 16, 2*time.Millisecond)

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
	rs := startReadSeeker(c, 10, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 3, filter, 4096, 2*time.Millisecond)

	assertCollected(c, buffer, []string{"Delta", "Hotel", "Juliet"}, nil)

	sigc <- struct{}{}

	assertCollected(c, buffer, []string{"Mike", "November", "Quebec", "Romeo", "Sierra", "Whiskey", "Yankee"}, nil)

	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestStop(c *gc.C) {
	// Stop after collected 10 lines.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 3, nil, 4096, 2*time.Millisecond)
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
	// Generate a read error after collected 10 lines.
	buffer := bytes.NewBuffer(nil)
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, sigc)

	<-sigc

	t := tailer.NewTailer(rs, buffer, 3, nil, 4096, 2*time.Millisecond)
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
					c.Assert(lines[i], gc.Equals, compare[i]+"\n")
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
func startReadSeeker(c *gc.C, initialLeg int, sigc chan struct{}) *readSeeker {
	// Write initial lines into the buffer.
	var rs readSeeker
	var i int
	for i = 0; i < initialLeg; i++ {
		rs.writeln(tailerData[i])
	}

	sigc <- struct{}{}

	// Continue with the rest in the background.
	go func() {
		<-sigc

		for ; i < len(tailerData); i++ {
			time.Sleep(5 * time.Millisecond)
			rs.writeln(tailerData[i])
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

func (r *readSeeker) writeln(s string) {
	r.mux.Lock()
	defer r.mux.Unlock()
	r.buffer = append(r.buffer, []byte(s)...)
	r.buffer = append(r.buffer, '\n')
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
	"Alpha",
	"Bravo",
	"Charlie",
	"Delta",
	"Echo",
	"Foxtrott",
	"Golf",
	"Hotel",
	"India",
	"Juliet",
	"Kilo",
	"Lima",
	"Mike",
	"November",
	"Oscar",
	"Papa",
	"Quebec",
	"Romeo",
	"Sierra",
	"Tango",
	"Uniform",
	"Victor",
	"Whiskey",
	"X-ray",
	"Yankee",
	"Zulu",
}
