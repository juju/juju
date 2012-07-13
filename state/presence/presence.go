// The presence package is intended as a replacement for zookeeper ephemeral
// nodes; the primary difference is that node timeout is unrelated to session
// timeout, and this allows us to restart a presence-enabled process "silently"
// (from the perspective of the rest of the system) without dealing with the
// complication of session re-establishment.

package presence

import (
	"fmt"
	zk "launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/state/watcher"
	"launchpad.net/tomb"
	"time"
)

// changeNode wraps a zookeeper node and can induce watches on that node to fire.
type changeNode struct {
	conn    *zk.Conn
	path    string
	content string
}

// change sets the zookeeper node's content (creating it if it doesn't exist) and
// returns the node's new MTime.
func (n *changeNode) change() (mtime time.Time, err error) {
	stat, err := n.conn.Set(n.path, n.content, -1)
	if zk.IsError(err, zk.ZNONODE) {
		_, err = n.conn.Create(n.path, n.content, 0, zk.WorldACL(zk.PERM_ALL))
		if err == nil || zk.IsError(err, zk.ZNODEEXISTS) {
			// *Someone* created the node anyway; just try again.
			return n.change()
		}
	}
	if err != nil {
		return
	}
	return stat.MTime(), nil
}

// Pinger periodically updates a node in zookeeper while it is running.
type Pinger struct {
	conn   *zk.Conn
	target changeNode
	period time.Duration
	tomb   tomb.Tomb
}

// StartPinger returns a new Pinger that refreshes the content of path periodically.
func StartPinger(conn *zk.Conn, path string, period time.Duration) (*Pinger, error) {
	target := changeNode{conn, path, period.String()}
	_, err := target.change()
	if err != nil {
		return nil, err
	}
	p := &Pinger{conn: conn, target: target, period: period}
	go p.loop()
	return p, nil
}

// loop calls change on p.target every p.period nanoseconds until p is stopped.
func (p *Pinger) loop() {
	defer p.tomb.Done()
	for {
		select {
		case <-p.tomb.Dying():
			return
		case <-time.After(p.period):
			if _, err := p.target.change(); err != nil {
				p.tomb.Kill(err)
				return
			}
		}
	}
}

// Dying returns a channel that will be closed when the Pinger is no longer
// operating.
func (p *Pinger) Dying() <-chan struct{} {
	return p.tomb.Dying()
}

// Stop stops updating the node; AliveW watches will not notice any change
// until they time out. A final write to the node is triggered to ensure
// watchers time out as late as possible.
func (p *Pinger) Stop() error {
	p.tomb.Kill(nil)
	if err := p.tomb.Wait(); err != nil {
		return err
	}
	_, err := p.target.change()
	return err
}

// Kill stops updating and deletes the node, causing any AliveW watches
// to observe its departure (almost) immediately.
func (p *Pinger) Kill() error {
	p.tomb.Kill(nil)
	if err := p.tomb.Wait(); err != nil {
		return err
	}
	return p.conn.Delete(p.target.path, -1)
}

// node represents the state of a presence node from a watcher's perspective.
type node struct {
	conn    *zk.Conn
	path    string
	alive   bool
	timeout time.Duration
}

// setState sets the current values of n.alive and n.timeout, given the
// content and stat of the zookeeper node (which must exist). firstTime
// should be true iff n.setState has not already been called.
func (n *node) setState(content string, stat *zk.Stat, firstTime bool) error {
	// Always check and reset timeout; it could change.
	period, err := time.ParseDuration(content)
	if err != nil {
		return fmt.Errorf("%s presence node has bad data: %q", n.path, content)
	}
	n.timeout = period * 2
	if firstTime {
		clock := changeNode{n.conn, "/clock", ""}
		now, err := clock.change()
		if err != nil {
			return err
		}
		delay := now.Sub(stat.MTime())
		n.alive = delay < n.timeout
	} else {
		// If this method is not being run for the first time, we know that
		// the node has just changed, so we know that it's alive and there's
		// no need to check for staleness with the clock node.
		n.alive = true
	}
	return nil
}

// update reads from ZooKeeper the current values of n.alive and n.timeout.
func (n *node) update() error {
	content, stat, err := n.conn.Get(n.path)
	if zk.IsError(err, zk.ZNONODE) {
		n.alive = false
		n.timeout = 0
		return nil
	} else if err != nil {
		return err
	}
	return n.setState(content, stat, true)
}

// updateW reads from ZooKeeper the current values of n.alive and n.timeout,
// and returns a ZooKeeper watch that will be notified of data or existence
// changes. firstTime should be true iff n.updateW has not already been called.
func (n *node) updateW(firstTime bool) (<-chan zk.Event, error) {
	content, stat, watch, err := n.conn.GetW(n.path)
	if zk.IsError(err, zk.ZNONODE) {
		n.alive = false
		n.timeout = 0
		stat, watch, err = n.conn.ExistsW(n.path)
		if err != nil {
			return nil, err
		}
		if stat != nil {
			// Someone *just* created the node; try again.
			return n.updateW(firstTime)
		}
		return watch, nil
	} else if err != nil {
		return nil, err
	}
	return watch, n.setState(content, stat, firstTime)
}

// waitDead sends false to watch when the node is deleted, or when it has
// not been updated recently enough to still qualify as alive. zkWatch must
// be observing changes on the existent node at n.path.
func (n *node) waitDead(zkWatch <-chan zk.Event, watch chan bool) {
	for n.alive {
		select {
		case <-time.After(n.timeout):
			n.alive = false
		case event := <-zkWatch:
			if !event.Ok() {
				close(watch)
				return
			}
			switch event.Type {
			case zk.EVENT_DELETED:
				n.alive = false
			case zk.EVENT_CHANGED:
				var err error
				zkWatch, err = n.updateW(false)
				if err != nil {
					close(watch)
					return
				}
			default:
				panic(fmt.Errorf("Unexpected event: %v", event))
			}
		}
	}
	watch <- false
}

// waitAlive sends true to watch when the node is changed or created. zkWatch
// must be observing changes on the stale/nonexistent node at n.path.
func (n *node) waitAlive(zkWatch <-chan zk.Event, watch chan bool) {
	for !n.alive {
		event := <-zkWatch
		if !event.Ok() {
			close(watch)
			return
		}
		switch event.Type {
		case zk.EVENT_CREATED, zk.EVENT_CHANGED:
			n.alive = true
		case zk.EVENT_DELETED:
			// The pinger is still dead (just differently dead); start a new watch.
			var err error
			zkWatch, err = n.updateW(false)
			if err != nil {
				close(watch)
				return
			}
		default:
			panic(fmt.Errorf("Unexpected event: %v", event))
		}
	}
	watch <- true
}

// Alive returns whether the Pinger at the given path seems to be alive.
func Alive(conn *zk.Conn, path string) (bool, error) {
	n := &node{conn: conn, path: path}
	err := n.update()
	return n.alive, err
}

// AliveW returns whether the Pinger at the given path seems to be alive.
// It also returns a channel that will receive the new status when it changes.
// If an error is encountered after AliveW returns, the channel will be closed.
func AliveW(conn *zk.Conn, path string) (bool, <-chan bool, error) {
	n := &node{conn: conn, path: path}
	zkWatch, err := n.updateW(true)
	if err != nil {
		return false, nil, err
	}
	alive := n.alive
	watch := make(chan bool)
	if alive {
		go n.waitDead(zkWatch, watch)
	} else {
		go n.waitAlive(zkWatch, watch)
	}
	return alive, watch, nil
}

// WaitAlive blocks until the node at the given path
// has been recently pinged or a timeout occurs.
func WaitAlive(conn *zk.Conn, path string, timeout time.Duration) error {
	alive, watch, err := AliveW(conn, path)
	if err != nil {
		return err
	}
	if alive {
		return nil
	}
	select {
	case alive, ok := <-watch:
		if !ok {
			return fmt.Errorf("presence: channel closed while waiting")
		}
		if !alive {
			return fmt.Errorf("presence: alive watch misbehaved while waiting")
		}
	case <-time.After(timeout):
		return fmt.Errorf("presence: still not alive after timeout")
	}
	return nil
}

// ChildrenWatcher mimics state/watcher's ChildrenWatcher, but treats nodes that
// do not have active Pingers as nonexistent.
type ChildrenWatcher struct {
	conn    *zk.Conn
	tomb    tomb.Tomb
	path    string
	alive   map[string]struct{}
	stops   map[string]chan struct{}
	updates chan aliveChange
	changes chan watcher.ChildrenChange
}

// aliveChange is used internally by ChildrenWatcher to communicate node
// status changes from childLoop goroutines to the loop goroutine.
type aliveChange struct {
	key   string
	alive bool
}

// NewChildrenWatcher returns a ChildrenWatcher that notifies of the
// presence and absence of Pingers in the direct child nodes of path.
func NewChildrenWatcher(conn *zk.Conn, path string) *ChildrenWatcher {
	w := &ChildrenWatcher{
		conn:    conn,
		path:    path,
		alive:   make(map[string]struct{}),
		stops:   make(map[string]chan struct{}),
		updates: make(chan aliveChange),
		changes: make(chan watcher.ChildrenChange),
	}
	go w.loop()
	return w
}

// Changes returns a channel on which presence changes can be received.
// The first event returns the current set of children on which Pingers
// are active.
func (w *ChildrenWatcher) Changes() <-chan watcher.ChildrenChange {
	return w.changes
}

// Stop terminates the watcher and returns any error encountered while watching.
func (w *ChildrenWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// Err returns the error that stopped the watcher, or
// tomb.ErrStillAlive if the watcher is still running.
func (w *ChildrenWatcher) Err() error {
	return w.tomb.Err()
}

func (w *ChildrenWatcher) loop() {
	defer w.finish()
	cw := watcher.NewChildrenWatcher(w.conn, w.path)
	defer watcher.Stop(cw, &w.tomb)
	emittedValue := false
	for {
		var change watcher.ChildrenChange
		select {
		case <-w.tomb.Dying():
			return
		case ch, ok := <-cw.Changes():
			var err error
			if !ok {
				err = watcher.MustErr(cw)
			} else {
				change, err = w.changeWatches(ch)
			}
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			if emittedValue && len(change.Added) == 0 && len(change.Removed) == 0 {
				continue
			}
		case ch, ok := <-w.updates:
			if !ok {
				panic("updates channel closed")
			}
			if ch.alive {
				w.alive[ch.key] = struct{}{}
				change = watcher.ChildrenChange{Added: []string{ch.key}}
			} else if _, alive := w.alive[ch.key]; alive {
				delete(w.alive, ch.key)
				change = watcher.ChildrenChange{Removed: []string{ch.key}}
			} else {
				// The node is already known to be dead, as a result of a
				// child removal detected by cw, and no further action need
				// be taken.
				continue
			}
		}
		select {
		case <-w.tomb.Dying():
			return
		case w.changes <- change:
			emittedValue = true
		}
	}
}

func (w *ChildrenWatcher) finish() {
	for _, stop := range w.stops {
		close(stop)
	}
	close(w.changes)
	w.tomb.Done()
}

// changeWatches starts new presence watches on newly-added candidates, and
// stops them on deleted ones. It returns a ChildrenChange representing only
// those changes that correspond to the presence or hitherto-undetected
// absence of a Pinger on a candidate node.
func (w *ChildrenWatcher) changeWatches(ch watcher.ChildrenChange) (watcher.ChildrenChange, error) {
	change := watcher.ChildrenChange{}
	for _, key := range ch.Removed {
		stop := w.stops[key]
		delete(w.stops, key)
		close(stop)
		// The node may already be known to be dead.
		if _, alive := w.alive[key]; alive {
			delete(w.alive, key)
			change.Removed = append(change.Removed, key)
		}
	}
	for _, key := range ch.Added {
		path := w.path + "/" + key
		alive, aliveW, err := AliveW(w.conn, path)
		if err != nil {
			return watcher.ChildrenChange{}, err
		}
		if alive {
			w.alive[key] = struct{}{}
			change.Added = append(change.Added, key)
		}
		stop := make(chan struct{})
		w.stops[key] = stop
		go w.childLoop(key, path, aliveW, stop)
	}
	return change, nil
}

// childLoop sends aliveChange events to w.updates, in response to presence
// changes received from watch, until its stop chan is closed.
func (w *ChildrenWatcher) childLoop(key, path string, watch <-chan bool, stop <-chan struct{}) {
	for {
		select {
		case <-stop:
			return
		case alive, ok := <-watch:
			if !ok {
				w.tomb.Killf("presence watch on %q failed", path)
				return
			}
			var aliveNow bool
			var err error
			aliveNow, watch, err = AliveW(w.conn, path)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			if aliveNow != alive {
				// state changed an odd number of times since the watch fired.
				continue
			}
			select {
			case <-stop:
				return
			case w.updates <- aliveChange{key, aliveNow}:
			}
		}
	}
}
