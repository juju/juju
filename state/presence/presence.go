package presence

import (
	"fmt"
	zk "launchpad.net/gozk/zookeeper"
	"time"
)

// changeNode wraps a zookeeper node and can induce watches on that node to fire.
type changeNode struct {
	conn *zk.Conn
	path string
	data string
}

// Change sets the zookeeper node's data (creating it if it doesn't exist) and
// returns the node's new MTime. This allows it to act as an ad-hoc remote clock
// in addition to its primary purpose of triggering watches on the node.
func (n *changeNode) Change() (mtime time.Time, err error) {
	stat, err := n.conn.Set(n.path, n.data, -1)
	if err == zk.ZNONODE {
		_, err = n.conn.Create(n.path, n.data, 0, zk.WorldACL(zk.PERM_ALL))
		if err == nil || err == zk.ZNODEEXISTS {
			// *Someone* created the node anyway; just try again.
			return n.Change()
		}
	}
	if err != nil {
		return
	}
	return stat.MTime(), nil
}

// Pinger continually updates a target node in zookeeper when run.
type Pinger struct {
	conn    *zk.Conn
	target  changeNode
	period  time.Duration
	closing chan bool
	closed  chan bool
}

// run calls Change on p.target every p.period nanoseconds until p is closed.
func (p *Pinger) run() {
	t := time.NewTicker(p.period)
	defer t.Stop()
	for {
		select {
		case <-p.closing:
			close(p.closed)
			return
		case <-t.C:
			_, err := p.target.Change()
			if err != nil {
				close(p.closed)
				<-p.closing
				return
			}
		}
	}
}

// Close just stops updating the target node; AliveW watches will not notice
// any change until they time out.
func (p *Pinger) Close() {
	p.closing <- true
	<-p.closed
}

// Kill stops updating and deletes the target node, causing any AliveW watches
// to observe its departure (almost) immediately.
func (p *Pinger) Kill() {
	p.Close()
	p.conn.Delete(p.target.path, -1)
}

// StartPing creates and returns an active Pinger, refreshing the contents of
// path every period nanoseconds.
func StartPing(conn *zk.Conn, path string, period time.Duration) (*Pinger, error) {
	target := changeNode{conn, path, period.String()}
	_, err := target.Change()
	if err != nil {
		return nil, err
	}
	p := &Pinger{conn, target, period, make(chan bool), make(chan bool)}
	go p.run()
	return p, nil
}

// pstate holds information about a remote Pinger's state.
type pstate struct {
	path    string
	alive   bool
	timeout time.Duration
}

// getPstate gets the latest known state of a remote Pinger, given the stat and
// content of its target node. path is present only for convenience's sake; this
// function is *not* responsible for acquiring stat and data itself, because its
// clients may or may not require a watch on the data; however, conn is still
// required, so that a clock node can be created and used to check staleness.
func getPstate(conn *zk.Conn, path string, stat *zk.Stat, data string) (pstate, error) {
	clock := changeNode{conn, "/clock", ""}
	now, err := clock.Change()
	if err != nil {
		return pstate{}, err
	}
	delay := now.Sub(stat.MTime())
	period, err := time.ParseDuration(data)
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
	data, stat, err := conn.Get(path)
	if err == zk.ZNONODE {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	p, err := getPstate(conn, path, stat, data)
	if err != nil {
		return false, err
	}
	return p.alive, err
}

// getPstateW gets the latest known state of a remote Pinger targeting path, and
// also returns a zookeeper watch which will fire on changes to the target node.
func getPstateW(conn *zk.Conn, path string) (p pstate, zkWatch <-chan zk.Event, err error) {
	data, stat, zkWatch, err := conn.GetW(path)
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
	p, err = getPstate(conn, path, stat, data)
	return
}

// awaitDead sends false to watch when the target node is deleted, or when it has
// not been updated recently enough to still qualify as alive.
func awaitDead(conn *zk.Conn, p pstate, zkWatch <-chan zk.Event, watch chan bool) {
	dead := time.After(p.timeout)
	select {
	case <-dead:
		watch <- false
	case event := <-zkWatch:
		if !event.Ok() {
			close(watch)
			return
		}
		switch event.Type {
		case zk.EVENT_DELETED:
			watch <- false
		case zk.EVENT_CHANGED:
			p, zkWatch, err := getPstateW(conn, p.path)
			if err != nil {
				close(watch)
				return
			}
			if p.alive {
				go awaitDead(conn, p, zkWatch, watch)
			} else {
				watch <- false
			}
		}
	}
}

// awaitAlive send true to watch when the target node is changed or created.
func awaitAlive(conn *zk.Conn, p pstate, zkWatch <-chan zk.Event, watch chan bool) {
	event := <-zkWatch
	if !event.Ok() {
		close(watch)
		return
	}
	switch event.Type {
	case zk.EVENT_CREATED, zk.EVENT_CHANGED:
		watch <- true
	case zk.EVENT_DELETED:
		// The pinger is still dead (just differently dead); start a new watch.
		p, zkWatch, err := getPstateW(conn, p.path)
		if err != nil {
			close(watch)
			return
		}
		if p.alive {
			watch <- true
		} else {
			go awaitAlive(conn, p, zkWatch, watch)
		}
	}
}

// AliveW returns the latest known liveness of a remote Pinger targeting path,
// and a one-shot channel by which the caller will be notified of the first
// liveness change to be detected.
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
