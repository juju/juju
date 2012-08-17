// The presence package is intended as a replacement for zookeeper ephemeral
// nodes; the primary difference is that node timeout is unrelated to session
// timeout, and this allows us to restart a presence-enabled process "silently"
// (from the perspective of the rest of the system) without dealing with the
// complication of session re-establishment.

package presence

import (
	"fmt"
	zk "launchpad.net/gozk/zookeeper"
	"launchpad.net/juju-core/log"
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

// Pinger allows a client to control the existence and refreshment of a
// presence node. Pingers can be started whenever they are stopped, and
// stopped whenever they are not killed.
type Pinger struct {
	conn     *zk.Conn
	target   changeNode
	period   time.Duration
	killed   bool
	tomb     *tomb.Tomb
	lastPing time.Time
}

// NewPinger returns a stopped Pinger.
func NewPinger(conn *zk.Conn, path string, period time.Duration) *Pinger {
	return &Pinger{
		conn:   conn,
		target: changeNode{conn, path, period.String()},
		period: period,
	}
}

// StartPinger returns a started Pinger.
func StartPinger(conn *zk.Conn, path string, period time.Duration) (*Pinger, error) {
	p := NewPinger(conn, path, period)
	if err := p.Start(); err != nil {
		return nil, err
	}
	return p, nil
}

// Start begins periodically refreshing the context of the Pinger's path.
// Consecutive calls to Start will panic, and will attempts to Start a
// Pinger that has been Killed.
func (p *Pinger) Start() error {
	if p.tomb != nil {
		panic("pinger is already started")
	}
	if p.killed {
		panic("pinger has been killed")
	}
	_, err := p.target.change()
	if err != nil {
		return err
	}
	p.lastPing = time.Now()
	p.tomb = &tomb.Tomb{}
	go p.loop()
	return nil
}

// Stop stops the periodic writes to the target node, if they are occurring.
// If this is the case, a final write to the node will be triggered, so that
// watchers will time out as late as possible.
func (p *Pinger) Stop() error {
	if p.killed {
		panic("pinger has been killed")
	}
	if p.tomb != nil {
		p.tomb.Kill(nil)
		err := p.tomb.Wait()
		p.tomb = nil
		return err
	}
	return nil
}

// Kill deletes the Pinger's target node, immediately signalling permanent
// departure to any watchers. Once the Pinger has been killed it must not
// be used again.
func (p *Pinger) Kill() error {
	if p.killed {
		return nil
	}
	if err := p.Stop(); err != nil {
		return err
	}
	p.killed = true
	err := p.conn.Delete(p.target.path, -1)
	if zk.IsError(err, zk.ZNONODE) {
		err = nil
	}
	return err
}

// loop calls change on p.target every p.period nanoseconds until p is stopped.
func (p *Pinger) loop() {
	defer p.tomb.Done()
	defer func() {
		// If we haven't encountered an error, always send a final write, to
		// ensure watchers detect the pinger's death as late as possible.
		if p.tomb.Err() == nil {
			_, err := p.target.change()
			p.tomb.Kill(err)
		}
	}()
	for {
		select {
		case <-p.tomb.Dying():
			log.Debugf("presence: %s pinger died after %s wait", p.target.path, time.Now().Sub(p.lastPing))
			return
		case now := <-p.nextPing():
			log.Debugf("presence: %s pinger awakened after %s wait", p.target.path, time.Now().Sub(p.lastPing))
			p.lastPing = now
			mtime, err := p.target.change()
			if err != nil {
				p.tomb.Kill(err)
				return
			}
			log.Debugf("presence: wrote to %s at (zk) %s", p.target.path, mtime)
		}
	}
}

// nextPing returns a channel that will receive an event when the pinger is next
// due to fire.
func (p *Pinger) nextPing() <-chan time.Time {
	next := p.lastPing.Add(p.period)
	wait := next.Sub(time.Now())
	if wait <= 0 {
		wait = 0
	}
	log.Debugf("presence: anticipating ping of %s in %s", p.target.path, wait)
	return time.After(wait)
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
	// Worst observed combination of GC pauses and scheduler weirdness led to
	// a gap between "500ms" pings of >1000ms; timeout should comfortably beat
	// that. This is largely an artifact of the notably short periods used in
	// tests; actual distributed Pingers should use longer periods in order to
	// compensate for network flakiness... and to allow a comfortable buffer
	// for pinger re-establishment by a replacement process when (eg when
	// upgrading agent binaries).
	n.timeout = period * 3
	if firstTime {
		clock := changeNode{n.conn, "/clock", ""}
		now, err := clock.change()
		if err != nil {
			return err
		}
		mtime := stat.MTime()
		delay := now.Sub(mtime)
		n.alive = delay < n.timeout
		log.Debugf(`
presence: initial diagnosis of %s
  now (zk)          %s
  last write (zk)   %s
  apparent delay    %s
  timeout           %s
  alive             %t
`[1:], n.path, now, mtime, delay, n.timeout, n.alive)
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
			if err := n.update(); err != nil {
				close(watch)
				return
			}
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
	alive   map[string]bool
	stops   map[string]chan bool
	updates chan aliveChange
	changes chan watcher.ChildrenChange
}

// aliveChange is used internally by ChildrenWatcher to communicate node
// status changes from childLoop goroutines to the main loop goroutine.
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
		alive:   make(map[string]bool),
		stops:   make(map[string]chan bool),
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
				w.alive[ch.key] = true
				change = watcher.ChildrenChange{Added: []string{ch.key}}
			} else if w.alive[ch.key] {
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

// finish tidies up any active watches on the child nodes, closes
// channels, and marks the watcher as finished.
func (w *ChildrenWatcher) finish() {
	for _, stop := range w.stops {
		stop <- true
	}
	// No need to close w.updates.
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
		stop <- true
		// The node might not already be known to be dead.
		if w.alive[key] {
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
		stop := make(chan bool)
		w.stops[key] = stop
		go w.childLoop(key, path, aliveW, stop)
		if alive {
			w.alive[key] = true
			change.Added = append(change.Added, key)
		}
	}
	return change, nil
}

// childLoop sends aliveChange events to w.updates, in response to presence
// changes received from watch (which is refreshed internally as required),
// until its stop chan is closed.
func (w *ChildrenWatcher) childLoop(key, path string, watch <-chan bool, stop <-chan bool) {
	for {
		select {
		case <-stop:
			return
		case alive, ok := <-watch:
			if !ok {
				w.tomb.Killf("presence watch on %q failed", path)
				return
			}
			// We definitely need to watch again; do so early, so we can verify
			// that the state has changed an odd number of times since we last
			// notified, and thereby only send notifications on real changes.
			aliveNow, newWatch, err := AliveW(w.conn, path)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
			watch = newWatch
			if aliveNow != alive {
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
