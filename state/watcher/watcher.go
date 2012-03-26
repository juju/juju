// launchpad.net/juju/state/watcher
//
// Copyright (c) 2011-2012 Canonical Ltd.

package watcher

import (
	"fmt"
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/tomb"
	"reflect"
	"sort"
)

const (
	ContentChange = iota
	ChildrenChange
)

// Watcher observes a ZooKeeper node for changes of the content or the
// children and delivers those via the Change() method.
type Watcher struct {
	zk         *zookeeper.Conn
	path       string
	changeType int
	changeChan chan []string
	buffer     []string
	tomb       tomb.Tomb
}

// NewWatcher creates a new watcher.
func NewWatcher(zk *zookeeper.Conn, path string, changeType int) (*Watcher, error) {
	if changeType != ContentChange && changeType != ChildrenChange {
		return nil, fmt.Errorf("watcher: illegal watcher type")
	}
	w := &Watcher{
		zk:         zk,
		path:       path,
		changeType: changeType,
		changeChan: make(chan []string),
	}
	go w.loop()
	return w, nil
}

// Changes delivers the observed changes as string slice
// containing the node content as first element or its 
// children as elements.
func (w *Watcher) Changes() <-chan []string {
	return w.changeChan
}

// Stop ends the watching.
func (w *Watcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// loop is the backend for watching.
func (w *Watcher) loop() {
	defer w.tomb.Done()
	defer close(w.changeChan)

	watch := singleEventChan(zookeeper.EVENT_CHANGED)
	for {
		select {
		case <-w.tomb.Dying():
			return
		case evt := <-watch:
			if !evt.Ok() {
				w.tomb.Killf("watcher: ZooKeeper critical session event: %v", evt)
				return
			}
			var cont bool
			switch w.changeType {
			case ContentChange:
				watch, cont = w.watchContent()
			case ChildrenChange:
				watch, cont = w.watchChildren()
			}
			if !cont {
				return
			}
		}
	}
}

// watchContent retrieves a node content and returns the next
// ZooKeeper watch.
func (w *Watcher) watchContent() (<-chan zookeeper.Event, bool) {
	content, _, nextWatch, err := w.zk.GetW(w.path)
	if err != nil {
		w.tomb.Kill(err)
		return nil, false
	}
	return nextWatch, w.update([]string{content})
}

// watchChildren retrieves the children of a node and returns the next
// ZooKeeper watch.
func (w *Watcher) watchChildren() (<-chan zookeeper.Event, bool) {
	children, _, nextWatch, err := w.zk.ChildrenW(w.path)
	sort.Strings(children)
	if err != nil {
		w.tomb.Kill(err)
		return nil, false
	}
	return nextWatch, w.update(children)
}

// update checks if the observed data has changed. Only in case 
// of a change the new data will be sent to the receiver.
func (w *Watcher) update(buffer []string) bool {
	if reflect.DeepEqual(buffer, w.buffer) {
		return true
	}
	firstContentCall := w.changeType == ContentChange && w.buffer == nil
	w.buffer = buffer
	if firstContentCall {
		return true
	}
	select {
	case <-w.tomb.Dying():
		return false
	case w.changeChan <- w.buffer:
	}
	return true
}

// singleEventChan creates the initial event channel and
// fires an event to read the first data.
func singleEventChan(etype int) <-chan zookeeper.Event {
	eventChan := make(chan zookeeper.Event, 1)
	eventChan <- zookeeper.Event{
		State: zookeeper.STATE_CONNECTED,
		Type:  etype,
	}
	return eventChan
}
