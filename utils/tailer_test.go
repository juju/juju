// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package utils_test

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/utils"
)

type tailerSuite struct{}

var _ = gc.Suite(tailerSuite{})

func (tailerSuite) TestMoreThanTailed(c *gc.C) {
	// Tails asks for less lines than exist.
	buffer := bytes.NewBuffer([]byte{})
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, 5*time.Millisecond, sigc)

	wait(c, sigc)

	t := utils.StartTailer(rs, 3, nil, 2*time.Millisecond, buffer)

	assertCollected(c, buffer, tailerData[2:5], nil, true)
	signal(c, sigc)
	assertCollected(c, buffer, tailerData[5:], nil, true)

	wait(c, sigc)
	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestLessThanTailed(c *gc.C) {
	// Tail asks initially for more lines than exist so far.
	buffer := bytes.NewBuffer([]byte{})
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 3, 5*time.Millisecond, sigc)

	wait(c, sigc)

	t := utils.StartTailer(rs, 5, nil, 2*time.Millisecond, buffer)

	assertCollected(c, buffer, tailerData[0:3], nil, true)
	signal(c, sigc)
	assertCollected(c, buffer, tailerData[3:], nil, true)

	wait(c, sigc)
	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestFiltered(c *gc.C) {
	// Only return lines containing an 'a'.
	filter := func(line string) bool {
		return strings.ContainsAny(line, "Aa")
	}
	buffer := bytes.NewBuffer([]byte{})
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 10, 5*time.Millisecond, sigc)

	wait(c, sigc)

	t := utils.StartTailer(rs, 7, filter, 2*time.Millisecond, buffer)

	assertCollected(c, buffer, []string{"Alpha", "Bravo", "Charlie", "Delta", "India"}, nil, true)
	signal(c, sigc)
	assertCollected(c, buffer, []string{"Lima", "Oscar", "Papa", "Sierra", "Tango", "X-ray"}, nil, true)

	wait(c, sigc)
	c.Assert(t.Stop(), gc.IsNil)
}

func (tailerSuite) TestStop(c *gc.C) {
	// Stop after collected 10 lines.
	buffer := bytes.NewBuffer([]byte{})
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, 5*time.Millisecond, sigc)

	wait(c, sigc)

	t := utils.StartTailer(rs, 3, nil, 2*time.Millisecond, buffer)
	stopper := func(lines []string) {
		if len(lines) == 10 {
			c.Assert(t.Stop(), gc.IsNil)
		}
	}

	assertCollected(c, buffer, tailerData[2:5], stopper, true)
	signal(c, sigc)
	assertCollected(c, buffer, tailerData[5:], stopper, false)

	c.Assert(t.Err(), gc.IsNil)
}

func (tailerSuite) TestError(c *gc.C) {
	// Generate a read error after collected 10 lines.
	buffer := bytes.NewBuffer([]byte{})
	sigc := make(chan struct{}, 1)
	rs := startReadSeeker(c, 5, 5*time.Millisecond, sigc)

	wait(c, sigc)

	t := utils.StartTailer(rs, 3, nil, 2*time.Millisecond, buffer)
	disturber := func(lines []string) {
		if len(lines) == 10 {
			rs.setError(fmt.Errorf("ouch after 10 lines"))
		}
	}

	assertCollected(c, buffer, tailerData[2:5], disturber, true)
	signal(c, sigc)
	assertCollected(c, buffer, tailerData[5:], disturber, false)

	c.Assert(t.Err(), gc.ErrorMatches, "ouch after 10 lines")
}

func assertCollected(c *gc.C, buffer *bytes.Buffer, collected []string, addon func([]string), timeout bool) {
	start := time.Now()
	lines := []string{}
	for {
		line, err := buffer.ReadString('\n')
		if len(line) > 0 {
			lines = append(lines, line)
			if addon != nil {
				addon(lines)
			}
			if len(lines) == len(collected) {
				for i := 0; i < len(lines); i++ {
					c.Assert(lines[i], gc.Equals, collected[i]+"\n")
				}
				return
			}
		}
		if err == io.EOF {
			if time.Now().Sub(start) > longTimeout {
				if timeout {
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

const (
	shortTimeout = 50 * time.Millisecond
	longTimeout  = 250 * time.Millisecond
)

func startReadSeeker(c *gc.C, initialLeg int, delay time.Duration, sigc chan struct{}) *readSeeker {
	// Write initial lines into the buffer.
	var rs *readSeeker = new(readSeeker)
	var i int
	for i = 0; i < initialLeg; i++ {
		rs.writeln(tailerData[i])
	}
	signal(c, sigc)
	// Continue with the rest in the background.
	go func() {
		wait(c, sigc)
		for ; i < len(tailerData); i++ {
			time.Sleep(delay)
			rs.writeln(tailerData[i])
		}
		signal(c, sigc)
	}()
	return rs
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
	if r.pos == len(r.buffer) {
		return 0, io.EOF
	}
	available := len(r.buffer) - r.pos
	capacity := len(p)
	if capacity < available {
		available = capacity
	}
	newPos := r.pos + available
	copy(p, r.buffer[r.pos:newPos])
	r.pos = newPos
	return available, nil
}

func (r *readSeeker) Seek(offset int64, whence int) (ret int64, err error) {
	r.mux.Lock()
	defer r.mux.Unlock()
	var newPos int64
	if whence != 2 {
		return 0, fmt.Errorf("invalid whence: %d", whence)
	}
	newPos = int64(len(r.buffer)) + offset
	if newPos < 0 {
		return 0, fmt.Errorf("negative position: %d", newPos)
	}
	if newPos >= 1<<31 {
		return 0, fmt.Errorf("position out of range: %d", newPos)
	}
	r.pos = int(newPos)
	return newPos, nil
}

func signal(c *gc.C, sigc chan struct{}) {
	select {
	case sigc <- struct{}{}:
	case <-time.After(shortTimeout):
		c.Fatalf("timeout during signalling")
	}
}

func wait(c *gc.C, sigc chan struct{}) {
	select {
	case <-sigc:
	case <-time.After(shortTimeout):
		c.Fatalf("timeout during waiting")
	}
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
