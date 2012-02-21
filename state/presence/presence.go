package presence

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"time"
)

// changeNode wraps a zookeeper node and can induce watches on that node to fire.
type changeNode struct {
	conn *zookeeper.Conn
	path string
	data string
}

// Change sets the zookeeper node's data (creating it if it doesn't exist) and
// returns the node's new MTime. This allows it to act as an ad-hoc remote clock.
func (n *changeNode) Change() (mtime time.Time, err error) {
	stat, err := n.conn.Set(n.path, n.data, -1)
	if err == zookeeper.ZNONODE {
		_, err = n.conn.Create(n.path, n.data, 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
		if err == nil || err == zookeeper.ZNODEEXISTS {
			// *Someone* created the node anyway; just try again.
			return n.Change()
		}
	}
	if err != nil {
		return
	}
	return stat.MTime(), nil
}

// getClock returns a changeNode wrapping /clock.
func getClock(conn *zookeeper.Conn) *changeNode {
	return &changeNode{conn, "/clock", ""}
}

// oneShot sends a single true value to ch and closes it.
func oneShot(ch chan bool) {
	ch <- true
	close(ch)
}

// Pinger (in concert with Watcher) acts as a replacement for an ephemeral node
// in zookeeper.
type Pinger struct {
	conn    *zookeeper.Conn
	node    changeNode
	timeout time.Duration
	closing chan bool
	closed  chan bool
	Error   error
}

// StartPing creates and returns an active Pinger which will delete path when
// it's closed.
func StartPing(conn *zookeeper.Conn, path string, timeout time.Duration) (*Pinger, error) {
	n := changeNode{conn, path, timeout.String()}
	_, err := n.Change()
	if err != nil {
		return nil, err
	}
	p := &Pinger{conn, n, timeout, make(chan bool), make(chan bool), nil}
	go p.start()
	return p, nil
}

// start writes timeout to p.path every timeout/2 nanoseconds.
func (p *Pinger) start() {
	t := time.NewTicker(p.timeout / 2)
	defer t.Stop()
	for {
		select {
		case <-p.closing:
			p.conn.Delete(p.node.path, -1)
			p.Error = fmt.Errorf("closed on request")
		case <-t.C:
			_, p.Error = p.node.Change()
		}
		if p.Error != nil {
			oneShot(p.closed)
			return
		}
	}
}

// Close deletes the target node, causing any connected Watchers to observe its
// departure (almost) immediately.
func (p *Pinger) Close() {
	oneShot(p.closing)
	<-p.closed
}

// Watcher (in concert with Pinger) acts as a replacement for a watch on an
// ephemeral node in zookeeper.
type Watcher struct {
	conn    *zookeeper.Conn
	path    string
	timeout time.Duration
	closing chan bool
	closed  chan bool

	// Is the node currently alive, to the best of our knowledge?
	Alive bool
	// Receives the new value of Alive whenever it changes.
	C chan bool
	// If non-nil, Alive is valid.
	Error error
}

// Watch creates and returns a new Watcher for path.
func Watch(conn *zookeeper.Conn, path string) (*Watcher, error) {
	w := &Watcher{
		conn:    conn,
		path:    path,
		timeout: 0,
		closing: make(chan bool),
		closed:  make(chan bool),
		C:       make(chan bool),
	}
	if err := w.watch(true); err != nil {
		return nil, err
	}
	return w, nil
}

// Alive is a convenience function that wraps Watch, for use in situations where
// a client wants to know that a Pinger is alive but doesn't need to be notified
// if and when it dies.
func Alive(conn *zookeeper.Conn, path string) (bool, error) {
	w, err := Watch(conn, path)
	if err != nil {
		return false, err
	}
	w.Close()
	return w.Alive, nil
}

// Close stops watching the node.
func (w *Watcher) Close() {
	oneShot(w.closing)
	<-w.closed
}

// readTimeout attempts to read the timeout declared by the Pinger refreshing
// w.path.
func (w *Watcher) readTimeout(data string) (err error) {
	w.timeout, err = time.ParseDuration(data)
	if err != nil {
		err = fmt.Errorf("%s is not a valid presence node: %s", w.path, err)
	}
	return
}

// watch starts or restarts a watch on w.path, whether or not the node exists,
// and whether or not the remote Pinger is currently alive.
func (w *Watcher) watch(firstTime bool) error {
	data, stat, zkWatch, err := w.conn.GetW(w.path)
	if err == zookeeper.ZNONODE {
		stat, zkWatch, err = w.conn.ExistsW(w.path)
		if err != nil {
			return err
		}
		if stat != nil {
			// Whoops, the Pinger was *just* started; try again.
			return w.watch(firstTime)
		}
		w.dead(zkWatch)
		return nil
	} else if err != nil {
		return err
	}
	if err = w.readTimeout(data); err != nil {
		return err
	}

	mtime, err := getClock(w.conn).Change()
	if err != nil {
		return err
	}
	alive := mtime.Sub(stat.MTime()) < w.timeout
	if firstTime {
		// Set w.Alive before the forthcoming .update in .alive/.dead, to prevent
		// .update from sending an unwanted liveness value into .C.
		w.Alive = alive
	}
	if alive {
		w.alive(zkWatch)
	} else {
		w.dead(zkWatch)
	}
	return nil
}

// rewatch delegates to watch to set up appropriate watches after a change in
// remote state, and handles errors.
func (w *Watcher) rewatch() {
	err := w.watch(false)
	if err != nil {
		w.broken(err)
	}
}

// update pushes the current liveness of the remote Pinger into C, so long as
// it is different from the last known liveness.
func (w *Watcher) update(alive bool) {
	if w.Alive != alive {
		w.Alive = alive
		w.C <- alive
	}
}

// alive should be called when the remote Pinger is known to be alive. zkWatch
// must be a data watch on w.path.
func (w *Watcher) alive(zkWatch <-chan zookeeper.Event) {
	w.update(true)
	if zkWatch == nil {
		w.rewatch()
		return
	}
	go func() {
		timeout := time.After(w.timeout)
		select {
		case event := <-zkWatch:
			if !event.Ok() {
				w.broken(fmt.Errorf(event.String()))
			} else if event.Type == zookeeper.EVENT_DELETED {
				w.dead(nil)
			} else {
				w.rewatch()
			}
		case <-timeout:
			w.dead(nil)
		case <-w.closing:
			w.broken(fmt.Errorf("stopped on request"))
		}
	}()
}

// dead should be called when the remote Pinger is known to be dead. zkWatch can
// be either an existence or a data watch on w.path.
func (w *Watcher) dead(zkWatch <-chan zookeeper.Event) {
	w.update(false)
	if zkWatch == nil {
		w.rewatch()
		return
	}
	go func() {
		select {
		case event := <-zkWatch:
			if !event.Ok() {
				w.broken(fmt.Errorf(event.String()))
			} else {
				switch event.Type {
				case zookeeper.EVENT_CREATED, zookeeper.EVENT_CHANGED:
					w.rewatch()
				case zookeeper.EVENT_DELETED:
					w.dead(nil)
				}
			}
		case <-w.closing:
			w.broken(fmt.Errorf("stopped on request"))
		}
	}()
}

// broken should be called when w is no longer working correctly, for whatever
// reason.
func (w *Watcher) broken(err error) {
	close(w.C)
	w.Error = err
	oneShot(w.closed)
}
