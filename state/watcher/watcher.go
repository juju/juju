package watcher

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/tomb"
)

// ContentWatcher observes a ZooKeeper node and delivers a
// notification when a content change is detected.
type ContentWatcher struct {
	zk         *zookeeper.Conn
	path       string
	tomb       tomb.Tomb
	changeChan chan string
	content    string
}

// NewContentWatcher creates a ContentWatcher observing
// the ZooKeeper node at watchedPath.
func NewContentWatcher(zk *zookeeper.Conn, watchedPath string) *ContentWatcher {
	w := &ContentWatcher{
		zk:         zk,
		path:       watchedPath,
		changeChan: make(chan string),
	}
	go w.loop()
	return w
}

// Changes returns a channel that will receive the new node
// content when a change is detected. Note that multiple
// changes may be observed as a single event in the channel.
func (w *ContentWatcher) Changes() <-chan string {
	return w.changeChan
}

// Dying returns a channel that is closed when the
// watcher has stopped or is about to stop.
func (w *ContentWatcher) Dying() <-chan struct{} {
	return w.tomb.Dying()
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

	watch, err := w.update(zookeeper.EVENT_CHANGED)
	if watch == nil {
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
			if watch == nil {
				w.tomb.Kill(err)
				return
			}
		}
	}
}

// update retrieves the node content and emits it to the change
// channel if it has changed. It returns the next watch.
func (w *ContentWatcher) update(eventType int) (nextWatch <-chan zookeeper.Event, err error) {
	if eventType == zookeeper.EVENT_DELETED {
		return nil, fmt.Errorf("watcher: node %q has been deleted", w.path)
	}
	content, _, watch, err := w.zk.GetW(w.path)
	if err != nil {
		return nil, fmt.Errorf("watcher: can't get content of node %q: %v", w.path, err)
	}
	if content == w.content {
		return watch, nil
	}
	w.content = content
	select {
	case <-w.tomb.Dying():
		return nil, tomb.ErrDying
	case w.changeChan <- w.content:
	}
	return watch, nil
}

// ChildrenChange contains information about
// children that have been created or deleted.
type ChildrenChange struct {
	Added   []string
	Deleted []string
}

// ChildrenWatcher observes a ZooKeeper node and delivers a
// notification when child nodes are added or removed.
type ChildrenWatcher struct {
	zk         *zookeeper.Conn
	path       string
	tomb       tomb.Tomb
	changeChan chan ChildrenChange
	children   map[string]bool
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
func (w *ChildrenWatcher) Changes() <-chan ChildrenChange {
	return w.changeChan
}

// Dying returns a channel that is closed when the
// watcher has stopped or is about to stop.
func (w *ChildrenWatcher) Dying() <-chan struct{} {
	return w.tomb.Dying()
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
	if watch == nil {
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
			if watch == nil {
				w.tomb.Kill(err)
				return
			}
		}
	}
}

// update retrieves the node children and emits the added or deleted children to 
// the change channel if it has changed. It returns the next watch.
func (w *ChildrenWatcher) update(eventType int) (nextWatch <-chan zookeeper.Event, err error) {
	if eventType == zookeeper.EVENT_DELETED {
		return nil, fmt.Errorf("watcher: node %q has been deleted", w.path)
	}
	retrievedChildren, _, watch, err := w.zk.ChildrenW(w.path)
	if err != nil {
		return nil, fmt.Errorf("watcher: can't get children of node %q: %v", w.path, err)
	}
	children := make(map[string]bool)
	for _, child := range retrievedChildren {
		children[child] = true
	}
	var change ChildrenChange
	for child, _ := range w.children {
		if !children[child] {
			change.Deleted = append(change.Deleted, child)
			delete(w.children, child)
		}
	}
	for child, _ := range children {
		if !w.children[child] {
			change.Added = append(change.Added, child)
			w.children[child] = true
		}
	}
	if len(change.Deleted) == 0 && len(change.Added) == 0 {
		return watch, nil
	}
	select {
	case <-w.tomb.Dying():
		return nil, tomb.ErrDying
	case w.changeChan <- change:
	}
	return watch, nil
}
