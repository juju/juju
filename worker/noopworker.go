// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

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
