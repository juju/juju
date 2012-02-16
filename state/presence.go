package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"time"
)

// ChangeNode wraps a zookeeper node whose mtime will change every time Change
// is called. The node itself will always be empty, but data watches will fire
// on Change calls.
type ChangeNode struct {
	conn *zookeeper.Conn
	path string
}

func NewChangeNode(conn *zookeeper.Conn, path string) (*ChangeNode, error) {
	n := &ChangeNode{conn, path}
	stat, err := n.conn.Exists(n.path)
	if err != nil {
		return nil, err
	}
	if stat != nil {
		return n, nil
	}
	_, err = n.conn.Create(n.path, "", 0, zkPermAll)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// Change re-sets contents of the zookeeper node, and returns the (approximate)
// time-according-to-zookeeper (in ms) at which this happened. This will cause
// data watches on this node to fire.
func (n *ChangeNode) Change() (int64, error) {
	stat, err := n.conn.Set(n.path, "", -1)
	if err != nil {
		return 0, err
	}
	return stat.MTime(), nil
}

// presenceNode holds all the data used by both PresenceNode and PresenceNodeClient.
type presenceNode struct {
	conn    *zookeeper.Conn
	path    string
	timeout time.Duration
}

// PresenceNode is a replacement for a zookeeper ephemeral node; it can be used to
// signal the presence of (eg) an agent in the same sort of way, but the timeout is
// independent of zookeeper session timeout. This means that an agent can die, and
// be restarted by upstart, and start sending keepalive ticks again *without*
// either bouncing the session (and spuriously alerting watchers to the departure)
// or attempting to re-establish the session and reconstruct watch state (which is
// likely to become unpleasantly complex).
type PresenceNode presenceNode

func NewPresenceNode(conn *zookeeper.Conn, path string, timeout time.Duration) *PresenceNode {
	return &PresenceNode{conn, path, timeout}
}

func (p *PresenceNode) Occupy() chan<- bool {
	done := make(chan bool, 1)
	go func() {
		n, err := NewChangeNode(p.conn, p.path)
		if err != nil {
			panic(fmt.Sprintf("cannot occupy presence node at %s", p.path))
		}
		t := time.NewTicker(p.timeout / 2)
		defer t.Stop()
		for {
			select {
			case <-done:
				p.conn.Delete(p.path, -1)
				return
			case <-t.C:
				n.Change()
			}
		}
	}()
	return done
}

// PresenceNodeClient allows a remote zookeeper client to detect and watch the
// occupation and vacation of a PresenceNode.
type PresenceNodeClient presenceNode

func NewPresenceNodeClient(conn *zookeeper.Conn, path string, timeout time.Duration) *PresenceNodeClient {
	return &PresenceNodeClient{conn, path, timeout}
}

// Occupied returns whether a presence node is currently occupied.
func (p *PresenceNodeClient) Occupied(clock *ChangeNode) (occupied bool, err error) {
	_, occupied, err = p.occupied(clock)
	return
}

// OccupiedW returns whether a presence node is currently occupied, and a
// channel which will receive a single bool when occupation status changes.
func (p *PresenceNodeClient) OccupiedW(clock *ChangeNode) (bool, <-chan bool, error) {
	exists, occupied, err := p.occupied(clock)
	if err != nil {
		return false, nil, err
	}
	if occupied {
		return p.occupiedW()
	}
	if exists {
		return p.unoccupiedW()
	}
	return p.nonexistentW()
}

// occupied returns the node's existence and occupation status. The clock param
// is necessary to allow a client to determine whether a node is occupied without
// watching and waiting for a timeout (as in occupiedW); it lets us find out
// (approximately) what time is "now", according to zookeeper, so we can compare
// that against the presence node's modified time and detect a node which has
// not been keepalive'd recently enough to qualify as occupied.
func (p *PresenceNodeClient) occupied(clock *ChangeNode) (exists bool, occupied bool, err error) {
	stat, err := p.conn.Exists(p.path)
	if err != nil || stat == nil {
		return
	}
	exists = true
	mtime, err := clock.Change()
	if err != nil {
		return
	}
	deltams := mtime - stat.MTime()
	occupied = time.Duration(deltams*1e6) < p.timeout
	return
}

// occupiedW does the work of OccupiedW when the node was already occupied.
func (p *PresenceNodeClient) occupiedW() (bool, <-chan bool, error) {
	occupied := make(chan bool, 1)

	go func() {
		for {
			timeout := make(chan bool, 1)
			go func(ch chan bool) {
				time.Sleep(p.timeout)
				ch <- true
			}(timeout)

			_, _, watch, err := p.conn.GetW(p.path)
			if err != nil {
				if err.(zookeeper.Error) == zookeeper.ZNONODE {
					occupied <- false
				}
				return
			}
			select {
			case event := <-watch:
				if event.Ok() {
					if event.Type == zookeeper.EVENT_DELETED {
						occupied <- false
						return
					}
				} else {
					panic("zookeeper connection down")
				}
			case <-timeout:
				occupied <- false
				return
			}
		}
	}()

	return true, occupied, nil
}

// unoccupiedW does the work of OccupiedW when we're waiting for the node to
// become occupied.
func (p *PresenceNodeClient) unoccupiedW() (bool, <-chan bool, error) {
	_, _, eWatch, err := p.conn.GetW(p.path)
	if err != nil {
		return false, nil, err
	}
	return false, waitFor(eWatch), nil
}

// nonexistentW does the work of OccupiedW when the node doesn't exist yet.
func (p *PresenceNodeClient) nonexistentW() (bool, <-chan bool, error) {
	// note: node could have been created in the meantime
	stat, eWatch, err := p.conn.ExistsW(p.path)
	if err != nil {
		return false, nil, err
	}
	return stat != nil, waitFor(eWatch), nil
}

func waitFor(eWatch <-chan zookeeper.Event) <-chan bool {
	oWatch := make(chan bool, 1)
	go func() {
		event := <-eWatch
		if event.Ok() {
			switch event.Type {
			case zookeeper.EVENT_CREATED, zookeeper.EVENT_CHANGED:
				oWatch <- true
			case zookeeper.EVENT_DELETED:
				oWatch <- false
			}
		} else {
			panic("zookeeper connection down")
		}
	}()
	return oWatch
}
