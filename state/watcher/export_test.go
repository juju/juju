package watcher

import (
	"time"
)

func FakePeriod(p time.Duration) {
	period = p
}

var realPeriod = period

func RealPeriod() {
	period = realPeriod
}
