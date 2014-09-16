package worker

func NewNoOpWorker() Worker {
	return NewSimpleWorker(doNothing)
}

func doNothing(stop <-chan struct{}) error {
	select {
	case <-stop:
		return nil
	}
}
