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
// returns the node's new MTime. This allows it to act as an ad-hoc remote clock
// in addition to its primary purpose of triggering watches on the node.
func (n *changeNode) change() (mtime time.Time, err error) {
	stat, err := n.conn.Set(n.path, n.content, -1)
	if err == zk.ZNONODE {
		_, err = n.conn.Create(n.path, n.content, 0, zk.WorldACL(zk.PERM_ALL))
		if err == nil || err == zk.ZNODEEXISTS {
			// *Someone* created the node anyway; just try again.
			return n.change()
		}
	}
	if err != nil {
		return
	}
	return stat.MTime(), nil
}

// Pinger continually updates a node in zookeeper when run.
type Pinger struct {
	conn    *zk.Conn
	target  changeNode
	period  time.Duration
	closing chan bool
}

// run calls change on p.target every p.period nanoseconds until p is closed.
func (p *Pinger) run() {
	for {
		select {
		case <-p.closing:
			return
		case <-time.After(p.period):
			_, err := p.target.change()
			if err != nil {
				<-p.closing
				return
			}
		}
	}
}

// Close stops updating the node; AliveW watches will not notice any change
// until they time out. A final write to the node is triggered to ensure
// watchers time out as late as possible.
func (p *Pinger) Close() {
	p.closing <- true
	p.target.change()
}

// Kill stops updating and deletes the node, causing any AliveW watches
// to observe its departure (almost) immediately.
func (p *Pinger) Kill() {
	p.closing <- true
	p.conn.Delete(p.target.path, -1)
}

// StartPinger creates and returns an active Pinger, refreshing the contents of
// path every period nanoseconds.
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

// update sets the current values of n.alive and n.timeout.
func (n *node) update() error {
	content, stat, err := n.conn.Get(n.path)
	if err == zk.ZNONODE {
		n.alive = false
		n.timeout = time.Duration(0)
		return nil
	} else if err != nil {
		return err
	}
	return n.updateExists(content, stat)
}

// updateW sets the current values of n.alive and n.timeout, and returns a
// zookeeper watch that will be notified of data or existence changes.
func (n *node) updateW() (<-chan zk.Event, error) {
	content, stat, watch, err := n.conn.GetW(n.path)
	if err == zk.ZNONODE {
		n.alive = false
		n.timeout = time.Duration(0)
		stat, watch, err = n.conn.ExistsW(n.path)
		if err != nil {
			return nil, err
		}
		if stat != nil {
			// Someone *just* created the node; try again.
			return n.updateW()
		}
		return watch, nil
	} else if err != nil {
		return nil, err
	}
	return watch, n.updateExists(content, stat)
}

// updateExists sets the current values of n.alive and n.timeout, given the
// content and stat of the zookeeper node (which must exist).
func (n *node) updateExists(content string, stat *zk.Stat) error {
	clock := changeNode{n.conn, "/clock", ""}
	now, err := clock.change()
	if err != nil {
		return err
	}
	period, err := time.ParseDuration(content)
	if err != nil {
		err := fmt.Errorf("%s is not a valid presence node: %s", n.path, err)
		return err
	}
	n.timeout = period * 2
	delay := now.Sub(stat.MTime())
	n.alive = delay < n.timeout
	return nil
}

// waitDead sends false to watch when the node is deleted, or when it has
// not been updated recently enough to still qualify as alive. It should only be
// called when the node is known to be alive.
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
				zkWatch, err = n.updateW()
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

// waitAlive sends true to watch when the node is changed or created. It should
// only be called when the node is known to be dead.
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
			zkWatch, err = n.updateW()
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
	zkWatch, err := n.updateW()
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
