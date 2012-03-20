// The presence package is intended as a replacement for zookeeper ephemeral
// nodes; the primary difference is that node timeout is unrelated to session
// timeout, and this allows us to restart a presence-enabled process "silently"
// (from the perspective of the rest of the system) without dealing with the
// complication of session re-establishment.

package presence

import (
	"fmt"
	zk "launchpad.net/gozk/zookeeper"
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
	conn     *zk.Conn
	target   changeNode
	period   time.Duration
	stopping chan bool
}

// run calls change on p.target every p.period nanoseconds until p is stopped.
func (p *Pinger) run() {
	for {
		select {
		case <-p.stopping:
			return
		case <-time.After(p.period):
			_, err := p.target.change()
			if err != nil {
				<-p.stopping
				return
			}
		}
	}
}

// Stop stops updating the node; AliveW watches will not notice any change
// until they time out. A final write to the node is triggered to ensure
// watchers time out as late as possible.
func (p *Pinger) Stop() {
	p.stopping <- true
	p.target.change()
}

// Kill stops updating and deletes the node, causing any AliveW watches
// to observe its departure (almost) immediately.
func (p *Pinger) Kill() {
	p.stopping <- true
	p.conn.Delete(p.target.path, -1)
}

// StartPinger returns a new Pinger that refreshes the content of path periodically.
func StartPinger(conn *zk.Conn, path string, period time.Duration) (*Pinger, error) {
	target := changeNode{conn, path, period.String()}
	_, err := target.change()
	if err != nil {
		return nil, err
	}
	p := &Pinger{conn, target, period, make(chan bool)}
	go p.run()
	return p, nil
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

// WaitAlive blocks until the Pinger at the given path is alive.
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
