package watcher

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/tomb"
)

// ErrStopper is implemented by all watchers.
type ErrStopper interface {
	Stop() error
	Err() error
}

// Stop stops the watcher. If an error is returned by the
// watcher, t is killed with the error.
func Stop(w ErrStopper, t *tomb.Tomb) {
	if err := w.Stop(); err != nil {
		t.Kill(err)
	}
}

// MustErr returns the error with which w died.
// Calling it will panic if w is still running or was stopped cleanly.
func MustErr(w ErrStopper) error {
	err := w.Err()
	if err == nil {
		panic("watcher was stopped cleanly")
	} else if err == tomb.ErrStillAlive {
		panic("watcher is still running")
	}
	return err
}

// ContentChange holds information on the existence
// and contents of a node. Version and Content will be
// zeroed when exists is false.
type ContentChange struct {
	Exists  bool
	Version int
	Content string
}

// ContentWatcher observes a ZooKeeper node and delivers a
// notification when a content change is detected.
type ContentWatcher struct {
	zk           *zookeeper.Conn
	path         string
	tomb         tomb.Tomb
	changeChan   chan ContentChange
	emittedValue bool
	content      ContentChange
}

// NewContentWatcher creates a ContentWatcher observing
// the ZooKeeper node at watchedPath.
func NewContentWatcher(zk *zookeeper.Conn, watchedPath string) *ContentWatcher {
	w := &ContentWatcher{
		zk:         zk,
		path:       watchedPath,
		changeChan: make(chan ContentChange),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the new node
// content when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
// The first event on the channel holds the initial state.
func (w *ContentWatcher) Changes() <-chan ContentChange {
	return w.changeChan
}

// Dying returns a channel that is closed when the
// watcher has stopped or is about to stop.
func (w *ContentWatcher) Dying() <-chan struct{} {
	return w.tomb.Dying()
}

// Err returns the error that stopped the watcher, or
// tomb.ErrStillAlive if the watcher is still running.
func (w *ContentWatcher) Err() error {
	return w.tomb.Err()
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called before
// discarding the watcher.
func (w *ContentWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// loop is the backend for watching.
func (w *ContentWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	watch, err := w.update()
	if err != nil {
		w.tomb.Kill(err)
		return
	}

	for {
		select {
		case <-w.tomb.Dying():
			return
		case evt := <-watch:
			if !evt.Ok() {
				w.tomb.Killf("watcher: critical session event: %v", evt)
				return
			}
			watch, err = w.update()
			if err != nil {
				w.tomb.Kill(err)
				return
			}
		}
	}
}

// update retrieves the node content and emits it as well as an existence
// flag to the change channel if it has changed. It returns the next watch.
func (w *ContentWatcher) update() (nextWatch <-chan zookeeper.Event, err error) {
	var content string
	var stat *zookeeper.Stat
	// Repeat until we have a valid watch or an error.
	for {
		content, stat, nextWatch, err = w.zk.GetW(w.path)
		if err == nil {
			// Node exists, so leave the loop.
			break
		}
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			// Need a new watch to receive a signal when the node is created.
			stat, nextWatch, err = w.zk.ExistsW(w.path)
			if stat != nil {
				// Node has been created just before ExistsW(),
				// so call GetW() with new loop run again.
				continue
			}
			if err == nil {
				// Got a valid watch, so leave loop.
				break
			}
		}
		// Any other error during GetW() or ExistsW().
		return nil, fmt.Errorf("watcher: can't get content of node %q: %v", w.path, err)
	}
	newContent := ContentChange{}
	if stat != nil {
		newContent.Exists = true
		newContent.Version = stat.Version()
		newContent.Content = content
	}
	if w.emittedValue && newContent == w.content {
		return nextWatch, nil
	}
	w.content = newContent
	select {
	case <-w.tomb.Dying():
		return nil, tomb.ErrDying
	case w.changeChan <- w.content:
		w.emittedValue = true
	}
	return nextWatch, nil
}

// ChildrenChange contains information about
// children that have been added or removed.
type ChildrenChange struct {
	Added   []string
	Removed []string
}

// ChildrenWatcher observes a ZooKeeper node and delivers a
// notification when child nodes are added or removed.
type ChildrenWatcher struct {
	zk           *zookeeper.Conn
	path         string
	tomb         tomb.Tomb
	changeChan   chan ChildrenChange
	emittedValue bool
	children     map[string]bool
}

// NewChildrenWatcher creates a ChildrenWatcher observing
// the ZooKeeper node at watchedPath.
func NewChildrenWatcher(zk *zookeeper.Conn, watchedPath string) *ChildrenWatcher {
	w := &ChildrenWatcher{
		zk:         zk,
		path:       watchedPath,
		changeChan: make(chan ChildrenChange),
		children:   make(map[string]bool),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the changes
// performed to the set of children of the watched node.
// Note that multiple changes may be observed as a single
// event in the channel.
// The first event on the channel represents the initial
// state - the Added field will hold the children found
// when NewChildrenWatcher was called.
func (w *ChildrenWatcher) Changes() <-chan ChildrenChange {
	return w.changeChan
}

// Dying returns a channel that is closed when the
// watcher has stopped or is about to stop.
func (w *ChildrenWatcher) Dying() <-chan struct{} {
	return w.tomb.Dying()
}

// Err returns the error that stopped the watcher, or
// tomb.ErrStillAlive if the watcher is still running.
func (w *ChildrenWatcher) Err() error {
	return w.tomb.Err()
}

// Stop stops the watch and returns any error encountered
// while watching. This method should always be called before
// discarding the watcher.
func (w *ChildrenWatcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// loop is the backend for watching.
func (w *ChildrenWatcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	watch, err := w.update(zookeeper.EVENT_CHILD)
	if err != nil {
		w.tomb.Kill(err)
		return
	}

	for {
		select {
		case <-w.tomb.Dying():
			return
		case evt := <-watch:
			if !evt.Ok() {
				w.tomb.Killf("watcher: critical session event: %v", evt)
				return
			}
			watch, err = w.update(evt.Type)
			if err != nil {
				w.tomb.Kill(err)
				return
			}
		}
	}
}

// update retrieves the node children and emits the added or deleted children to 
// the change channel if it has changed. It returns the next watch.
func (w *ChildrenWatcher) update(eventType int) (nextWatch <-chan zookeeper.Event, err error) {
	retrievedChildren := []string{}
	for {
		retrievedChildren, _, nextWatch, err = w.zk.ChildrenW(w.path)
		if zookeeper.IsError(err, zookeeper.ZNONODE) {
			var stat *zookeeper.Stat
			if stat, nextWatch, err = w.zk.ExistsW(w.path); err != nil {
				return
			} else if stat != nil {
				// The node suddenly turns out to exist; try to get
				// a child watch again.
				continue
			}
			break
		}
		if err != nil {
			return
		}
		break
	}
	children := make(map[string]bool)
	for _, child := range retrievedChildren {
		children[child] = true
	}
	var change ChildrenChange
	for child, _ := range w.children {
		if !children[child] {
			change.Removed = append(change.Removed, child)
			delete(w.children, child)
		}
	}
	for child, _ := range children {
		if !w.children[child] {
			change.Added = append(change.Added, child)
			w.children[child] = true
		}
	}
	if w.emittedValue && len(change.Removed) == 0 && len(change.Added) == 0 {
		return
	}
	select {
	case <-w.tomb.Dying():
		return nil, tomb.ErrDying
	case w.changeChan <- change:
		w.emittedValue = true
	}
	return
}
