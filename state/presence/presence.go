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
	conn      *zk.Conn
	tomb      tomb.Tomb
	target    changeNode
	period    time.Duration
	lastPing  time.Time
	stayAlive chan struct{}
}

// StartPinger returns a new Pinger that refreshes the content of path periodically.
func StartPinger(conn *zk.Conn, path string, period time.Duration) (*Pinger, error) {
	target := changeNode{conn, path, period.String()}
	_, err := target.change()
	if err != nil {
		return nil, err
	}
	p := &Pinger{
		conn:      conn,
		target:    target,
		period:    period,
		lastPing:  time.Now(),
		stayAlive: make(chan struct{}),
	}
	go p.loop()
	return p, nil
}

// loop calls change on p.target every p.period nanoseconds until p is stopped.
func (p *Pinger) loop() {
	defer p.finish()
	tick := p.getTick()
	for {
		select {
		case <-p.tomb.Dying():
			log.Debugf("presence: %s pinger died after %s wait", p.target.path, time.Now().Sub(p.lastPing))
			return
		case now := <-tick:
			log.Debugf("presence: %s pinger awakened after %s wait", p.target.path, time.Now().Sub(p.lastPing))
			p.lastPing = now
			mtime, err := p.target.change()
			if err != nil {
				p.tomb.Kill(err)
				return
			}
			log.Debugf("presence: wrote to %s at (zk) %s", p.target.path, mtime)
			tick = p.getTick()
		}
	}
}

// getTick returns a channel that will receive an event when the pinger is next
// due to fire.
func (p *Pinger) getTick() <-chan time.Time {
	next := p.lastPing.Add(p.period)
	wait := next.Sub(time.Now())
	if wait <= 0 {
		wait = 0
	}
	log.Debugf("presence: anticipating ping of %s in %s", p.target.path, wait)
	return time.After(wait)
}

// finish marks the Pinger as finally dead; it may also trigger a final write
// to the target node, if the Pinger has been cleanly shut down via the Stop
// method.
func (p *Pinger) finish() {
	if p.tomb.Err() == nil {
		select {
		case <-p.stayAlive:
			if _, err := p.target.change(); err != nil {
				p.tomb.Kill(err)
			}
		default:
		}
	}
	p.tomb.Done()
}

// Dying returns a channel that will be closed when the Pinger is no longer
// operating.
func (p *Pinger) Dying() <-chan struct{} {
	return p.tomb.Dying()
}

// Stop stops updating the node; AliveW watches will not notice any change
// until they time out. If the pinger is shut down without errors, a final
// write to the target node will occur, to ensure that remote watchers take
// as long as possible to detect the Pinger's absence.
func (p *Pinger) Stop() error {
	close(p.stayAlive)
	p.tomb.Kill(nil)
	return p.tomb.Wait()
}

// Kill stops updating and deletes the node, causing any AliveW watches
// to observe its departure (almost) immediately.
func (p *Pinger) Kill() (err error) {
	p.tomb.Kill(nil)
	if err = p.tomb.Wait(); err == nil {
		if err = p.conn.Delete(p.target.path, -1); zk.IsError(err, zk.ZNONODE) {
			return nil
		}
	}
	return
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
	// for Pinger re-establishment by a replacement process when (eg when
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
