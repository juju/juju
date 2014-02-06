package peergrouper_test

import (
	gc "launchpad.net/gocheck"
	"launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/worker"
	"launchpad.net/juju-core/worker/peergrouper"
)

type workerSuite struct {
	testing.JujuConnSuite
}

var _ = gc.Suite(&workerSuite{})

func (s *workerSuite) TestStartStop(c *gc.C) {
	w, err := peergrouper.New(s.State)
	c.Assert(err, gc.IsNil)
	err = worker.Stop(w)
	c.Assert(err, gc.IsNil)
}

func (s *workerSuite) 



type fakeState struct {
	mu sync.Mutex
	machines map[string] *machineDoc
}

type fakeMachine struct {
	
}

type fakeSession struct {
}

// notifier implements a value that can be
// watched for changes. Only one 
type notifier struct {
	mu sync.Mutex
	val *voyeur.Value
}

func newNotifier() *notifier {
	return &notifier{
		val: voyeur.NewValue(struct{}),
	}
}

func (n *notifier) watch() state.NotifyWatcher  {
	return 
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.watching {
		panic("double watch")
	}
	n.notify = make(chan struct{}, 1)
	n.watching = true
	return &notifyWatcher{
		stopped: make(chan struct{}),
		notify: n.notify,
	}
}

func (n *notifier) changed() {
	select {
	case n.c <- struct{}:
	default:
	}
}

type notifyWatcher struct {
	stopped chan struct{}
	notify <-chan struct{}
}

func (w *notifyWatcher) Kill() {
	defer func() { recover() }
	close(w.stopped)
}

func (w *notifyWatcher) Wait() error {
	<-w.stopped
	return nil
}

func (w *notifyWatcher) Stop() error {
	w.Kill()
	return w.Wait()
}

func (w *notifyWatcher) Err() error {
	return nil
}

func (w *notifyWatcher) Changes() <-chan struct{} {
	return w.notify
}
