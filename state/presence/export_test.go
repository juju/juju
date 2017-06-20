// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package presence

import (
	"time"

	"github.com/juju/errors"

	"github.com/juju/juju/testing"
)

func FakeTimeSlot(offset int) {
	fakeTimeSlot(offset)
}

func RealTimeSlot() {
	realTimeSlot()
}

func FakePeriod(seconds int64) {
	period = seconds
}

var realPeriod = period

func RealPeriod() {
	period = realPeriod
}

func (pb *PingBatcher) ForceFlush() error {
	request := make(chan struct{})
	select {
	case pb.flushChan <- request:
		select {
		case <-request:
			return nil
		case <-pb.tomb.Dying():
			return pb.tomb.Err()
		case <-time.After(testing.LongWait):
			return errors.Errorf("timeout waiting for flush to finish")
		}
	case <-pb.tomb.Dying():
		return pb.tomb.Err()
	case <-time.After(testing.LongWait):
		return errors.Errorf("timeout waiting for flush request")
	}
}
