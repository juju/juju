package presence

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

func FindAllBeings(w *Watcher) (map[int64]beingInfo, error) {
    return w.findAllBeings()
}
