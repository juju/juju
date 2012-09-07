// The watcher package implements an interface for observing changes
// to arbitrary MongoDB documents that are maintained via the
// mgo/txn transaction package.
package watcher

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"time"
)

// A Watcher can watch any number of collections and documents for changes.
type Watcher struct {
	tomb tomb.Tomb
	log  *mgo.Collection

	// watches holds the observers managed by Watch/Unwatch.
	watches map[watchKey][]watchInfo

	// current holds the current txn-revno values for all the observed
	// documents known to exist. Documents not observed or deleted are
	// omitted from this map and are considered to have revno -1.
	current map[watchKey]int64

	// refreshEvents and requestEvents contain the events to be
	// dispatched to the watcher channels. They're queued during
	// processing and flushed at the end to simplify the algorithm.
	// The two queues are separated because events from refresh are
	// handled in reverse order due to the way the algorithm works.
	refreshEvents, requestEvents []event

	// request is used to deliver requests from the public API into
	// the the gorotuine loop.
	request chan interface{}

	// refreshed contains pending ForceRefresh done channels
	// that are waiting for the completion notice.
	refreshed []chan bool

	// lastId is the most recent transaction id observed by a refresh.
	lastId interface{}

	// next will dispatch when it's time to refresh the database
	// knowledge. It's maintained here so that ForceRefresh
	// can manipulate it to force a refresh sooner.
	next <-chan time.Time
}

// A Change holds information about a document change.
type Change struct {
	// C and Id hold the collection name and document _id field value.
	C  string
	Id interface{}

	// Revno is the latest known value for the document's txn-revno
	// field, or -1 if the document was deleted.
	Revno int64
}

type watchKey struct {
	c  string
	id interface{}
}

type watchInfo struct {
	ch    chan<- Change
	revno int64
}

type event struct {
	ch    chan<- Change
	key   watchKey
	revno int64
}

// New returns a new Watcher observing the changelog collection.
// That collection must be maintained by the mgo/txn package.
func New(changelog *mgo.Collection) *Watcher {
	w := &Watcher{
		log:     changelog,
		watches: make(map[watchKey][]watchInfo),
		current: make(map[watchKey]int64),
		request: make(chan interface{}),
	}
	go func() {
		w.tomb.Kill(w.loop())
		w.tomb.Done()
	}()
	return w
}

// Stop stops all the watcher activities.
func (w *Watcher) Stop() error {
	w.tomb.Kill(nil)
	return w.tomb.Wait()
}

type reqWatch struct {
	key  watchKey
	info watchInfo
}

type reqUnwatch struct {
	key watchKey
	ch  chan<- Change
}

type reqRefresh struct {
	done chan bool
}

// Watch includes the given collection and document id into w for monitoring.
// An event will be sent onto ch whenever its txn-revno is observed to
// change after a transaction is applied.
// The revno parameter informs the currently known revision number for the
// document, or -1 if it doesn't currently exist.
func (w *Watcher) Watch(collection string, id interface{}, revno int64, ch chan<- Change) {
	select {
	case w.request <- reqWatch{watchKey{collection, id}, watchInfo{ch, revno}}:
	case <-w.tomb.Dying():
	}
}

// Unwatch removes from w the given collection, document id, and channel.
func (w *Watcher) Unwatch(collection string, id interface{}, ch chan<- Change) {
	select {
	case w.request <- reqUnwatch{watchKey{collection, id}, ch}:
	case <-w.tomb.Dying():
	}
}

// ForceRefresh forces a synchronous refresh of the watcher knowledge.
// It blocks until the database state has been loaded and the events
// have been prepared, but unblocks before changes are sent onto the
// registered channels.
func (w *Watcher) ForceRefresh() {
	done := make(chan bool)
	select {
	case w.request <- reqRefresh{done}:
	case <-w.tomb.Dying():
	}
	select {
	case <-done:
	case <-w.tomb.Dying():
	}
}

// period is the length of each time slot in seconds.
// It's not a time.Duration because the code is more convenient like
// this and also because sub-second timings don't work as the slot
// identifier is an int64 in seconds.
var period int64 = 30

// loop implements the main watcher loop.
func (w *Watcher) loop() error {
	w.next = time.After(0)
	if err := w.initLastId(); err != nil {
		return err
	}
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case <-w.next:
			w.next = time.After(time.Duration(period) * time.Second)
			refreshed := w.refreshed
			w.refreshed = nil
			if err := w.refresh(); err != nil {
				return err
			}
			for _, done := range refreshed {
				close(done)
			}
		case req := <-w.request:
			w.handle(req)
		}
		w.flush()
	}
	return nil
}

// flush sends all pending events to their respective channels.
func (w *Watcher) flush() {
	// Refresh events are handled backwards, to prevent moving
	// significant data around and still preserve occurrence order.
	i := len(w.refreshEvents)-1
	for i >= 0 {
		e := &w.refreshEvents[i]
		for e.ch != nil {
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
			}
			break
		}
		i--
	}
	// Request events are handled forwards.
	i = 0
	for i < len(w.requestEvents) {
		e := &w.requestEvents[i]
		for e.ch != nil {
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case e.ch <- Change{e.key.c, e.key.id, e.revno}:
			}
			break
		}
		i++
	}
	w.refreshEvents = w.refreshEvents[:0]
	w.requestEvents = w.requestEvents[:0]
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *Watcher) handle(req interface{}) {
	log.Debugf("watcher: got request: %#v", req)
	switch r := req.(type) {
	case reqRefresh:
		w.next = time.After(0)
		w.refreshed = append(w.refreshed, r.done)
	case reqWatch:
		for _, info := range w.watches[r.key] {
			if info.ch == r.info.ch {
				panic("adding channel twice for same document")
			}
		}
		if revno, ok := w.current[r.key]; ok && (revno > r.info.revno || revno == -1 && r.info.revno >= 0) {
			r.info.revno = revno
			w.requestEvents = append(w.requestEvents, event{r.info.ch, r.key, revno})
		}
		w.watches[r.key] = append(w.watches[r.key], r.info)
	case reqUnwatch:
		watches := w.watches[r.key]
		for i, info := range watches {
			if info.ch == r.ch {
				watches[i] = watches[len(watches)-1]
				w.watches[r.key] = watches[:len(watches)-1]
				break
			}
		}
		for i := range w.requestEvents {
			e := &w.requestEvents[i]
			if e.key == r.key && e.ch == r.ch {
				e.ch = nil
			}
		}
		for i := range w.refreshEvents {
			e := &w.refreshEvents[i]
			if e.key == r.key && e.ch == r.ch {
				e.ch = nil
			}
		}
	default:
		panic(fmt.Errorf("unknown request: %T", req))
	}
}

type logInfo struct {
	Docs   []interface{} `bson:"d"`
	Revnos []int64       `bson:"r"`
}

// initLastId reads the most recent changelog document and initializes
// lastId with it. This causes all history that precedes the creation
// of the watcher to be ignored.
func (w *Watcher) initLastId() error {
	log.Debugf("watcher: reading most recent document to ignore past history...")
	var entry struct {
		Id interface{} "_id"
	}
	err := w.log.Find(nil).Sort("-$natural").One(&entry)
	if err != nil && err != mgo.ErrNotFound {
		return err
	}
	w.lastId = entry.Id
	return nil
}

// refresh updates the watcher knowledge from the database, and
// queues events to observing channels.
func (w *Watcher) refresh() error {
	log.Debugf("watcher: refreshing watcher knowledge from database...")
	iter := w.log.Find(nil).Batch(10).Sort("-$natural").Iter()
	seen := make(map[watchKey]bool)
	first := true
	lastId := w.lastId
	var entry bson.D
	for iter.Next(&entry) {
		if len(entry) == 0 {
			log.Debugf("watcher: got empty changelog document")
			continue
		}
		id := entry[0]
		if id.Name != "_id" {
			panic("watcher: _id field isn't first entry")
		}
		if first {
			w.lastId = id.Value
			first = false
		}
		if id.Value == lastId {
			break
		}
		log.Debugf("watcher: got changelog document: %#v", entry)
		for _, c := range entry {
			var d, r []interface{}
			dr, _ := c.Value.(bson.D)
			for _, item := range dr {
				switch item.Name {
				case "d":
					d, _ = item.Value.([]interface{})
				case "r":
					r, _ = item.Value.([]interface{})
				}
			}
			if len(d) == 0 || len(d) != len(r) {
				log.Printf("watcher: changelog has invalid collection document: %#v", c)
				continue
			}
			for i := len(d) - 1; i >= 0; i-- {
				key := watchKey{c.Name, d[i]}
				if seen[key] {
					continue
				}
				seen[key] = true
				revno, ok := r[i].(int64)
				if !ok {
					log.Printf("watcher: changelog has revno with type %T: %#v", r[i], r[i])
					continue
				}
				if revno < 0 {
					revno = -1
				}
				w.current[key] = revno
				infos := w.watches[key]
				for i, info := range infos {
					if revno > info.revno || revno < 0 && info.revno >= 0 {
						infos[i].revno = revno
						w.refreshEvents = append(w.refreshEvents, event{info.ch, key, revno})
					}
				}
			}
		}
	}
	if iter.Err() != nil {
		return fmt.Errorf("watcher iteration error: %v", iter.Err())
	}
	return nil
}
