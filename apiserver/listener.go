// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/juju/utils/clock"
)

func newThrottlingListener(inner net.Listener, minPause, maxPause time.Duration, clk clock.Clock) net.Listener {
	rand.Seed(time.Now().UTC().UnixNano())
	if clk == nil {
		clk = clock.WallClock
	}
	return &throttlingListener{
		Listener:        inner,
		maxPause:        maxPause,
		minPause:        minPause,
		clk:             clk,
		connAcceptTimes: make([]*time.Time, 500),
	}
}

type throttlingListener struct {
	sync.Mutex
	net.Listener
	connAcceptTimes []*time.Time
	nextSlot        int

	minPause time.Duration
	maxPause time.Duration
	clk      clock.Clock
}

// connRateMetric returns an int value based on the rate of new connections.
func (l *throttlingListener) connRateMetric() int {
	l.Lock()
	defer l.Unlock()

	var (
		earliestConnTime *time.Time
		connCount        float64
	)
	// Figure out the most recent connection timestamp.
	startIndex := l.nextSlot - 1
	if startIndex < 0 {
		startIndex = len(l.connAcceptTimes) - 1
	}
	latestConnTime := l.connAcceptTimes[startIndex]
	if latestConnTime == nil {
		return 0
	}
	// Loop backwards to get the earlier known connection timestamp.
	for index := startIndex; index != l.nextSlot; {
		if connTime := l.connAcceptTimes[index]; connTime == nil {
			break
		} else {
			earliestConnTime = connTime
		}
		connCount++
		index--
		if index < 0 {
			index = len(l.connAcceptTimes) - 1
		}
	}
	if connCount < 2 {
		return 0
	}
	// We use as a metric how many connections per 10ms
	connRate := connCount * float64(10*time.Millisecond) / (1.0 + float64(latestConnTime.Sub(*earliestConnTime)))
	logger.Tracef("server listener has received %d connections per 10ms", connRate)
	return int(connRate)
}

// Accept waits for and returns the next connection to the listener.
func (l *throttlingListener) Accept() (net.Conn, error) {
	l.pause()
	conn, err := l.Listener.Accept()
	if err == nil {
		l.Lock()
		defer l.Unlock()
		now := l.clk.Now()
		l.connAcceptTimes[l.nextSlot] = &now
		l.nextSlot++
		if l.nextSlot > len(l.connAcceptTimes)-1 {
			l.nextSlot = 0
		}
	}
	return conn, err
}

// pauseTime returns a random time based on rate of connections.
func (l *throttlingListener) pauseTime() time.Duration {
	// The pause time is minPause plus 5ms for each unit increase
	// in connection rate, up to a maximum of maxPause,
	wantedMaxPause := l.minPause + time.Duration(l.connRateMetric()*5)*time.Millisecond
	if wantedMaxPause > l.maxPause {
		return l.maxPause
	}
	if wantedMaxPause == l.minPause {
		return l.minPause
	}
	pauseTime := time.Duration(rand.Intn(int((wantedMaxPause-l.minPause)/time.Millisecond))) * time.Millisecond
	pauseTime += l.minPause
	return pauseTime
}

func (l *throttlingListener) pause() {
	if l.minPause <= 0 || l.maxPause <= 0 {
		return
	}
	pauseTime := l.pauseTime()
	select {
	case <-l.clk.After(pauseTime):
	}
}
