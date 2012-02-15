package state

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"strconv"
	"time"
)

type IncNode struct {
	conn *zookeeper.Conn
	path string
}

func NewIncNode(conn *zookeeper.Conn, path string) (*IncNode, error) {
	n := &IncNode{conn, path}
	stat, err := n.conn.Exists(n.path)
	if err != nil {
		return nil, err
	}
	if stat != nil {
		return n, nil
	}
	_, err = n.conn.Create(n.path, "0", 0, zkPermAll)
	if err != nil {
		return nil, err
	}
	return n, nil
}

// Inc returns the (zookeeper) time at which this increment happened.
func (n *IncNode) Inc() (int64, error) {
	data, stat, err := n.conn.Get(n.path)
	if err != nil {
		return 0, err
	}
	count, _ := strconv.Atoi(data)
	stat, err = n.conn.Set(n.path, fmt.Sprintf("%d", count+1), -1)
	if err != nil {
		return 0, err
	}
	return stat.MTime(), nil
}

type PresenceNode struct {
	conn    *zookeeper.Conn
	path    string
	timeout int64
}

func (p *PresenceNode) Occupy(done <-chan bool) {
	n, err := NewIncNode(p.conn, p.path)
	if err != nil {
		return
	}
	t := time.NewTicker(time.Duration(p.timeout / 2))
	for {
		select {
		case <-done:
			break
		case <-t.C:
			n.Inc()
		}
	}
	t.Stop()
}

// Occupied returns whether a presence node is currently occupied.
func (p *PresenceNode) Occupied(clock *IncNode) (occupied bool, err error) {
	_, occupied, err = p.occupied(clock)
	return
}

// OccupiedW returns whether a presence node is currently occupied, and a
// channel which will receive a single bool when occupation status changes.
func (p *PresenceNode) OccupiedW(clock *IncNode) (bool, <-chan bool, error) {
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

// occupied returns the node's existence and occupation status
func (p *PresenceNode) occupied(clock *IncNode) (exists bool, occupied bool, err error) {
	stat, err := p.conn.Exists(p.path)
	if err != nil || stat == nil {
		return
	}
	exists = true
	mtime, err := clock.Inc()
	if err != nil {
		return
	}
	occupied = (mtime - stat.MTime()) < p.timeout
	return
}

// occupiedW does the work of OccupiedW when the node was already occupied.
func (p *PresenceNode) occupiedW() (bool, <-chan bool, error) {
	occupied := make(chan bool, 1)
	timeouts := make(chan int, 1)
	lastTimeout := 0

	go func() {
		for {
			lastTimeout += 1
			go func(t int) {
				time.Sleep(time.Duration(p.timeout))
				timeouts <- t
			}(lastTimeout)

			_, _, watch, err := p.conn.GetW(p.path)
			if err != nil {
				if err.(zookeeper.Error) == zookeeper.ZNONODE {
					occupied <- false
				}
				return
			}
			event := <-watch
			if event.Ok() {
				if event.Type == zookeeper.EVENT_DELETED {
					occupied <- false
					break
				}
			} else {
				break
			}
		}
	}()
	go func() {
		for {
			if <-timeouts == lastTimeout {
				occupied <- false
			}
		}
	}()

	return true, occupied, nil
}

// unoccupiedW does the work of OccupiedW when we're waiting for the node to
// become occupied.
func (p *PresenceNode) unoccupiedW() (bool, <-chan bool, error) {
	_, _, eWatch, err := p.conn.GetW(p.path)
	if err != nil {
		return false, nil, err
	}
	return false, waitFor(eWatch), nil
}

// nonexistentW does the work of OccupiedW when the node doesn't exist yet.
func (p *PresenceNode) nonexistentW() (bool, <-chan bool, error) {
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
		}
	}()
	return oWatch
}
