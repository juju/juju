// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver

import (
	"net"
	"sync"
	"time"

	"github.com/juju/utils/clock"
)

func newThrottlingListener(inner net.Listener, cfg RateLimitConfig, clk clock.Clock) net.Listener {
	if clk == nil {
		clk = clock.WallClock
	}
	return &throttlingListener{
		Listener:        inner,
		maxPause:        cfg.ConnMaxPause,
		minPause:        cfg.ConnMinPause,
		lookbackWindow:  cfg.ConnLookbackWindow,
		lowerThreshold:  cfg.ConnLowerThreshold,
		upperThreshold:  cfg.ConnUpperThreshold,
		clk:             clk,
		connAcceptTimes: make([]*time.Time, 200),
	}
}

// throttlingListener wraps a net.Listener and throttles connection accepts
// based on the rate of incoming connections.
type throttlingListener struct {
	sync.Mutex
	net.Listener

	connAcceptTimes []*time.Time
	nextSlot        int
	clk             clock.Clock

	minPause       time.Duration
	maxPause       time.Duration
	lookbackWindow time.Duration
	lowerThreshold int
	upperThreshold int
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
		// Stop if we have reached the maximum window in terms how long
		// ago the earliest connection was, to avoid stale data skewing results.
		if latestConnTime.Sub(*earliestConnTime) > l.lookbackWindow {
			break
		}
		index--
		if index < 0 {
			index = len(l.connAcceptTimes) - 1
		}
	}
	if connCount < 2 {
		return 0
	}
	// We use as a metric how many connections per 10ms
	connRate := connCount * float64(time.Second) / (1.0 + float64(latestConnTime.Sub(*earliestConnTime)))
	logger.Tracef("server listener has received %d connections per second", int(connRate))
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

// pauseTime returns a time based on rate of connections.
// - up to lowerThreshold, return minPause
// - above to upperThreshold, return maxPause
// - in between, return an interpolated value
func (l *throttlingListener) pauseTime() time.Duration {
	rate := l.connRateMetric()
	if rate <= l.lowerThreshold {
		return l.minPause
	}
	if rate >= l.upperThreshold {
		return l.maxPause
	}
	// rate is between min and max so interpolate.
	pauseFactor := float64(rate-l.lowerThreshold) / float64(l.upperThreshold-l.lowerThreshold)
	return l.minPause + time.Duration(float64(l.maxPause-l.minPause)*pauseFactor)
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
