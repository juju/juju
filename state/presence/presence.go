package presence

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"time"
)

// ChangeNode wraps a zookeeper node whose mtime will change every time Change
// is called. The actual node data will not change, but data watches will fire
// on Change calls.
type ChangeNode struct {
	conn *zookeeper.Conn
	path string
	data string
}

func NewChangeNode(conn *zookeeper.Conn, path string, data string) (*ChangeNode, error) {
	n := &ChangeNode{conn, path, data}
	stat, err := n.conn.Exists(n.path)
	if err != nil {
		return nil, err
	}
	if stat != nil {
		return n, nil
	}
	_, err = n.conn.Create(n.path, data, 0, zookeeper.WorldACL(zookeeper.PERM_ALL))
	if err != nil {
		return nil, err
	}
	return n, nil
}

// Change re-sets contents of the zookeeper node, and returns the (approximate)
// time-according-to-zookeeper at which this happened. This will cause
// data watches on this node to fire.
func (n *ChangeNode) Change() (time.Time, error) {
	fmt.Println("A")
	stat, err := n.conn.Set(n.path, n.data, -1)
	fmt.Println("B")
	if err != nil {
		return time.Unix(0, 0), err
	}
	return stat.MTime(), nil
}

type Event struct {
	// Is the node now occupied?
	Occupied bool
	// Non-nil if Occupied is invalid.
	Error error
}

type Node struct {
	done chan<- bool
}

func (n *Node) Vacate() {
	n.done <- true
}

// Occupy implements a replacement for a zookeeper ephemeral node; it can be used to
// signal the presence of (eg) an agent in the same sort of way, but the timeout is
// independent of zookeeper session timeout. This means that an agent can die, and
// be restarted by upstart, and start sending keepalive ticks again *without*
// either bouncing the session (and spuriously alerting watchers to the departure)
// or attempting to re-establish the session and reconstruct watch state (which is
// likely to become unpleasantly complex).
func Occupy(conn *zookeeper.Conn, path string, timeout time.Duration) (*Node, error) {
	n, err := NewChangeNode(conn, path, timeout.String())
	if err != nil {
		return nil, err
	}
	done := make(chan bool, 1)
	go func() {
		t := time.NewTicker(timeout / 2)
		defer t.Stop()
		for {
			select {
			case <-done:
				conn.Delete(path, -1)
				return
			case <-t.C:
				n.Change()
			}
		}
	}()
	return &Node{done}, nil
}

// Client allows a remote zookeeper client to detect and watch the occupation
// and vacation of a Node.
type Client struct {
	conn    *zookeeper.Conn
	path    string
	timeout time.Duration
}

func NewClient(conn *zookeeper.Conn, path string) *Client {
	return &Client{conn, path, 0}
}

// Occupied returns whether a presence node is currently occupied.
func (c *Client) Occupied(clock *ChangeNode) (occupied bool, err error) {
	occupied, _, err = c.occupied(clock)
	return
}

// OccupiedW returns whether a presence node is currently occupied, and a
// channel which will receive a single Event when occupation status changes.
func (c *Client) OccupiedW(clock *ChangeNode) (occupied bool, watch <-chan Event, err error) {
	occupied, zkWatch, err := c.occupied(clock)
	if err == nil {
		if occupied {
			watch = c.occupiedW(zkWatch)
		} else {
			watch = c.unoccupiedW(zkWatch)
		}
	}
	return
}

// readTimeout tries to parse the contents of a presence node as a Duration.
func (c *Client) readTimeout(data string) (err error) {
	c.timeout, err = time.ParseDuration(data)
	if err != nil {
		err = fmt.Errorf("%s is not a valid presence node: %s", c.path, err)
	}
	return
}

// occupied returns the node's existence and occupation status. The clock param
// is necessary to allow a client to determine whether a node is occupied without
// watching and waiting for a timeout (as in occupiedW); it lets us find out
// (approximately) what time is "now", according to zookeeper, so we can compare
// that against the presence node's modified time and detect a node which has
// not been keepalive'd recently enough to qualify as occupied.
func (c *Client) occupied(clock *ChangeNode) (occupied bool, watch <-chan zookeeper.Event, err error) {
	data, stat, watch, err := c.conn.GetW(c.path)
	if err == zookeeper.ZNONODE {
		stat, watch, err = c.conn.ExistsW(c.path)
		if stat != nil {
			// Whoops, the node was just created, try again from the top.
			return c.occupied(clock)
		}
		// All return values are set correctly, whether err == nil or not.
		return
	} else if err != nil {
		return
	}
	if err = c.readTimeout(data); err != nil {
		return
	}
	mtime, err := clock.Change()
	if err != nil {
		return
	}
	occupied = mtime.Sub(stat.MTime()) < c.timeout
	return
}

// occupiedW does the work of OccupiedW when the node was already occupied.
func (c *Client) occupiedW(watch <-chan zookeeper.Event) <-chan Event {
	occupied := make(chan Event, 1)
	go func() {
		for {
			timeout := time.After(c.timeout)
			select {
			case event := <-watch:
				if !event.Ok() {
					occupied <- Event{Error: fmt.Errorf(event.String())}
					return
				} else if event.Type == zookeeper.EVENT_DELETED {
					occupied <- Event{Occupied: false}
					return
				}
			case <-timeout:
				occupied <- Event{Occupied: false}
				return
			}

			var err error
			var data string
			data, _, watch, err = c.conn.GetW(c.path)
			if err != nil {
				if err == zookeeper.ZNONODE {
					occupied <- Event{Occupied: false}
				} else {
					occupied <- Event{Error: err}
				}
				return
			}
			if err = c.readTimeout(data); err != nil {
				occupied <- Event{Error: err}
				return
			}
		}
	}()
	return occupied
}

// unoccupiedW does the work of OccupiedW when we're waiting for the node to
// become occupied.
func (c *Client) unoccupiedW(watch <-chan zookeeper.Event) <-chan Event {
	occupied := make(chan Event, 1)
	go func() {
		event := <-watch
		if event.Ok() {
			switch event.Type {
			// Note: watch could have come from either GetW or ExistsW.
			case zookeeper.EVENT_CREATED, zookeeper.EVENT_CHANGED:
				occupied <- Event{Occupied: true}
			case zookeeper.EVENT_DELETED:
				occupied <- Event{Occupied: false}
			}
		} else {
			occupied <- Event{Error: fmt.Errorf(event.String())}
		}
	}()
	return occupied
}
