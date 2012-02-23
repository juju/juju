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

// pstate holds information about a remote Pinger's state.
type pstate struct {
	path    string
	alive   bool
	timeout time.Duration
}

// getPstate gets the latest known state of a remote Pinger, given the mtime and
// content of its target node. path is present only for convenience's sake; this
// function is *not* responsible for acquiring stat and content itself, because its
// clients may or may not require a watch on the node; however, conn is still
// required, so that a clock node can be created and used to check staleness.
func getPstate(conn *zk.Conn, path string, mtime time.Time, content string) (pstate, error) {
	clock := changeNode{conn, "/clock", ""}
	now, err := clock.change()
	if err != nil {
		return pstate{}, err
	}
	delay := now.Sub(mtime)
	period, err := time.ParseDuration(content)
	if err != nil {
		err := fmt.Errorf("%s is not a valid presence node: %s", path, err)
		return pstate{}, err
	}
	timeout := period * 2
	alive := delay < timeout
	return pstate{path, alive, timeout}, nil
}

// Alive returns whether a remote Pinger targeting path is alive.
func Alive(conn *zk.Conn, path string) (bool, error) {
	content, stat, err := conn.Get(path)
	if err == zk.ZNONODE {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	p, err := getPstate(conn, path, stat.MTime(), content)
	if err != nil {
		return false, err
	}
	return p.alive, err
}

// getPstateW gets the latest known state of a remote Pinger targeting path, and
// also returns a zookeeper watch which will fire on changes to the target node.
func getPstateW(conn *zk.Conn, path string) (p pstate, zkWatch <-chan zk.Event, err error) {
	content, stat, zkWatch, err := conn.GetW(path)
	if err == zk.ZNONODE {
		stat, zkWatch, err = conn.ExistsW(path)
		if err != nil {
			return
		}
		if stat != nil {
			// Whoops, node *just* appeared. Try again.
			return getPstateW(conn, path)
		}
		return
	} else if err != nil {
		return
	}
	p, err = getPstate(conn, path, stat.MTime(), content)
	return
}

// awaitDead sends false to watch when the node is deleted, or when it has
// not been updated recently enough to still qualify as alive. It should only be
// called when the node is known to be alive.
func awaitDead(conn *zk.Conn, p pstate, zkWatch <-chan zk.Event, watch chan bool) {
	for p.alive {
		select {
		case <-time.After(p.timeout):
			p.alive = false
		case event := <-zkWatch:
			if !event.Ok() {
				close(watch)
				return
			}
			switch event.Type {
			case zk.EVENT_DELETED:
				p.alive = false
			case zk.EVENT_CHANGED:
				var err error
				p, zkWatch, err = getPstateW(conn, p.path)
				if err != nil {
					close(watch)
					return
				}
			}
		}
	}
	watch <- false
}

// awaitAlive sends true to watch when the node is changed or created. It should
// only be called when the node is known to be dead.
func awaitAlive(conn *zk.Conn, p pstate, zkWatch <-chan zk.Event, watch chan bool) {
	for !p.alive {
		event := <-zkWatch
		if !event.Ok() {
			close(watch)
			return
		}
		switch event.Type {
		case zk.EVENT_CREATED, zk.EVENT_CHANGED:
			p.alive = true
		case zk.EVENT_DELETED:
			// The pinger is still dead (just differently dead); start a new watch.
			var err error
			p, zkWatch, err = getPstateW(conn, p.path)
			if err != nil {
				close(watch)
				return
			}
		}
	}
	watch <- true
}

// AliveW returns whether the Pinger at the given node path seems to be alive.
// It also returns a channel that will receive the new status when it changes.
// If an error is encountered after AliveW returns, the channel will be closed.
func AliveW(conn *zk.Conn, path string) (bool, <-chan bool, error) {
	p, zkWatch, err := getPstateW(conn, path)
	if err != nil {
		return false, nil, err
	}
	watch := make(chan bool)
	if p.alive {
		go awaitDead(conn, p, zkWatch, watch)
	} else {
		go awaitAlive(conn, p, zkWatch, watch)
	}
	return p.alive, watch, nil
}
