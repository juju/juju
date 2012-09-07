package watcher

func FakePeriod(seconds int64) {
	period = seconds
}

var realPeriod = period

func RealPeriod() {
	period = realPeriod
}
