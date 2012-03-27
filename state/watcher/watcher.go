package watcher

import (
	"launchpad.net/gozk/zookeeper"
	"launchpad.net/tomb"
	"sort"
)

// updater defines the interface that has to be implemented
// by the concrete watchers to handle and deliver node 
// updates individually.
type updater interface {
	// update checks if the observed data has changed. Only in case 
	// of a change the new data will be sent to a receiver. The methods
	// returns the next watch and a continuation flag to the watcher.
	// The latter will be set to false in case of an error or the
	// receiving of a stop signal.
	update() (nextWatch <-chan zookeeper.Event, cont bool)
	// close is called if the watcher ends its work.
	close()
}

// watcher provides the backend goroutine handling for the
// observation of ZooKeeper node changes. The concrete handling
// has to be done by the updater.
type watcher struct {
	tomb    tomb.Tomb
	updater updater
}

// init assigns the updater and then starts the watching loop.
func (w *watcher) init(updater updater) {
	w.updater = updater
	go w.loop()
}

// Stop ends the watching.
func (w *watcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

// loop is the backend for watching.
func (w *watcher) loop() {
	defer w.tomb.Done()
	defer w.updater.close()

	// Fire an initial event.
	watch := func() <-chan zookeeper.Event {
		eventChan := make(chan zookeeper.Event, 1)
		eventChan <- zookeeper.Event{
			State: zookeeper.STATE_CONNECTED,
			Type:  zookeeper.EVENT_CHANGED,
		}
		return eventChan
	}()

	for {
		select {
		case <-w.tomb.Dying():
			return
		case evt := <-watch:
			if !evt.Ok() {
				w.tomb.Killf("watcher: critical session event: %v", evt)
				return
			}
			var cont bool
			watch, cont = w.updater.update()
			if !cont {
				return
			}
		}
	}
}

// ContentWatcher observes a ZooKeeper node for changes of the content
// and delivers those via the Change() method.
type ContentWatcher struct {
	zk         *zookeeper.Conn
	path       string
	changeChan chan string
	content    string
	watcher
}

// NewContentWatcher creates a new content watcher.
func NewContentWatcher(zk *zookeeper.Conn, path string) (*ContentWatcher, error) {
	w := &ContentWatcher{
		zk:         zk,
		path:       path,
		changeChan: make(chan string),
	}
	w.watcher.init(w)
	return w, nil
}

// Changes emits the content of the node on the
// returned channel each time it changes.
func (w *ContentWatcher) Changes() <-chan string {
	return w.changeChan
}

// update is documented in the updater interface above. For the
// ContentWatcher it retrieves the nodes content as string and
// emits changed content via the changeChan.
func (w *ContentWatcher) update() (nextWatch <-chan zookeeper.Event, cont bool) {
	content, _, watch, err := w.zk.GetW(w.path)
	if err != nil {
		w.tomb.Kill(err)
		return nil, false
	}
	if content == w.content {
		return watch, true
	}
	w.content = content
	select {
	case <-w.tomb.Dying():
		return nil, false
	case w.changeChan <- w.content:
	}
	return watch, true
}

// close is documented in the updater interface above. For the
// ContentWatcher it just closes the changeChan.
func (w *ContentWatcher) close() {
	close(w.changeChan)
}

// ChildrenChange contains information about
// children that have been created or deleted.
type ChildrenChange struct {
	// Del holds names of children that have been deleted.
	Del []string
	// New holds names of children that have been created.
	New []string
}

// ChildrenWatcher observes a ZooKeeper node for changes of the
// children and delivers those via the Change() method.
type ChildrenWatcher struct {
	zk         *zookeeper.Conn
	path       string
	changeChan chan ChildrenChange
	children   map[string]bool
	watcher
}

// NewWatcher creates a new watcher.
func NewChildrenWatcher(zk *zookeeper.Conn, path string) (*ChildrenWatcher, error) {
	w := &ChildrenWatcher{
		zk:         zk,
		path:       path,
		changeChan: make(chan ChildrenChange),
		children:   make(map[string]bool),
	}
	w.watcher.init(w)
	return w, nil
}

// Changes emits the deleted and the new created children of 
// the node on the returned channel each time they are changing.
func (w *ChildrenWatcher) Changes() <-chan ChildrenChange {
	return w.changeChan
}

// update is documented in the updater interface above. For the
// ChildrenWatcher it retrieves the nodes children, checks which
// are added or deleted and emits these changes via the changeChan.
func (w *ChildrenWatcher) update() (nextWatch <-chan zookeeper.Event, cont bool) {
	children, _, watch, err := w.zk.ChildrenW(w.path)
	if err != nil {
		w.tomb.Kill(err)
		return nil, false
	}
	diff := make(map[string]bool)
	for _, child := range children {
		diff[child] = true
	}
	var change ChildrenChange
	for child, _ := range w.children {
		if !diff[child] {
			change.Del = append(change.Del, child)
			delete(w.children, child)
		}
	}
	for child, _ := range diff {
		if !w.children[child] {
			change.New = append(change.New, child)
			w.children[child] = true
		}
	}
	if len(change.Del) == 0 && len(change.New) == 0 {
		return watch, true
	}
	sort.Strings(change.Del)
	sort.Strings(change.New)
	select {
	case <-w.tomb.Dying():
		return nil, false
	case w.changeChan <- change:
	}
	return watch, true
}

// close is documented in the updater interface above. For the
// ChildrenWatcher it just closes the changeChan.
func (w *ChildrenWatcher) close() {
	close(w.changeChan)
}
