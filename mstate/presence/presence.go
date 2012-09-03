package presence

import (
	"fmt"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"launchpad.net/juju-core/log"
	"launchpad.net/tomb"
	"time"
)

// A Watcher can watch any number of pinger keys for liveness changes.
type Watcher struct {
	base     *mgo.Collection
	pings    *mgo.Collection
	beings   *mgo.Collection
	delta    time.Duration
	beingKey map[int64]string
	beingSeq map[string]int64
	watches  map[string][]chan<- Change
	request  chan interface{}
	tomb     tomb.Tomb
	next     <-chan time.Time
	pending  []event

	// refreshed contains pending ForcedRefresh done channels
	// that are waiting for the completion notice.
	refreshed []chan bool
}

type event struct {
	ch    chan<- Change
	key   string
	alive bool
}

// Change holds a liveness change notification.
type Change struct {
	Key   string
	Alive bool
}

// New returns a new Watcher.
func NewWatcher(base *mgo.Collection) *Watcher {
	w := &Watcher{
		base:     base,
		pings:    pingsC(base),
		beings:   beingsC(base),
		beingKey: make(map[int64]string),
		beingSeq: make(map[string]int64),
		watches:  make(map[string][]chan<- Change),
		request:  make(chan interface{}),
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

type reqAdd struct {
	key string
	ch  chan<- Change
}

type reqRemove struct {
	key string
	ch  chan<- Change
}

type reqRefresh struct {
	done chan bool
}

type reqAlive struct {
	key    string
	result chan bool
}

// Add includes key into w for liveness monitoring. An initial
// event will be sent onto ch to report the initially known status
// for key, and from then on a new event will be sent whenever a
// change is detected. Change values sent to the channel must be
// consumed, or the whole watcher will blocked.
func (w *Watcher) Add(key string, ch chan<- Change) {
	w.request <- reqAdd{key, ch}
}

// Remove removes key and ch from liveness monitoring.
func (w *Watcher) Remove(key string, ch chan<- Change) {
	w.request <- reqRemove{key, ch}
}

// ForceRefresh forces a synchronous refresh of the watcher knowledge.
// It blocks until the database state has been loaded and the events
// have been prepared, but it is fine to consume the events after
// ForceRefresh returns.
func (w *Watcher) ForceRefresh() {
	done := make(chan bool)
	w.request <- reqRefresh{done}
	select {
	case <-done:
	case <-w.tomb.Dying():
	}
}

// Alive returns whether the key is currently considered alive by w.
func (w *Watcher) Alive(key string) bool {
	result := make(chan bool)
	w.request <- reqAlive{key, result}
	var alive bool
	select {
	case alive = <-result:
	case <-w.tomb.Dying():
	}
	return alive
}

// period is the length of each time slot in seconds.
// It's not a time.Duration because the code is more convenient like
// this and also because sub-second timings don't work as the slot
// identifier is an int64 in seconds.
var period int64 = 30

// loop implements the main watcher loop.
func (w *Watcher) loop() error {
	var err error
	if w.delta, err = clockDelta(w.base); err != nil {
		return err
	}
	w.next = time.After(0)
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
	i := 0
	for i < len(w.pending) {
		e := &w.pending[i]
		if e.ch == nil {
			i++ // Removed meanwhile.
			continue
		}
		select {
		case <-w.tomb.Dying():
			return
		case req := <-w.request:
			w.handle(req)
		case e.ch <- Change{e.key, e.alive}:
			i++
		}
	}
	w.pending = w.pending[:0]
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *Watcher) handle(req interface{}) {
	switch r := req.(type) {
	case reqRefresh:
		w.next = time.After(0)
		w.refreshed = append(w.refreshed, r.done)
	case reqAdd:
		for _, ch := range w.watches[r.key] {
			if ch == r.ch {
				panic("adding channel twice for same key")
			}
		}
		w.watches[r.key] = append(w.watches[r.key], r.ch)
		_, alive := w.beingSeq[r.key]
		w.pending = append(w.pending, event{r.ch, r.key, alive})
	case reqRemove:
		watches := w.watches[r.key]
		for i, ch := range watches {
			if ch == r.ch {
				copy(watches[:len(watches)-1], watches[i+1:])
				w.watches[r.key] = watches[:len(watches)-1]
				break
			}
		}
		for i := range w.pending {
			e := &w.pending[i]
			if e.key == r.key && e.ch == r.ch {
				e.ch = nil
			}
		}
	case reqAlive:
		_, alive := w.beingSeq[r.key]
		select {
		case r.result <- alive:
		case <-w.tomb.Dying():
		}
	default:
		panic(fmt.Errorf("unknown request: %T", req))
	}
}

type beingInfo struct {
	Key string "_id,omitempty"
	Seq int64  "seq,omitempty"
}

type pingInfo struct {
	Slot  int64            "_id"
	Alive map[string]int64 ",omitempty"
	Dead  map[string]int64 ",omitempty"
}

// refresh updates the watcher knowledge from the database, and
// queues events to observing channels. The process is done by
// fetching the last two time slots, and comparing the union of
// both to the in-memory state.
func (w *Watcher) refresh() error {
	slot := timeSlot(time.Now(), w.delta)
	var ping []pingInfo
	err := w.pings.Find(bson.D{{"$or", []pingInfo{{Slot: slot}, {Slot: slot - period}}}}).All(&ping)
	if err != nil && err == mgo.ErrNotFound {
		return err
	}

	var k int64
	var being beingInfo

	// Learn about all enforced deaths.
	dead := make(map[int64]bool)
	for i := range ping {
		for key, value := range ping[i].Dead {
			if _, err := fmt.Sscanf(key, "%x", &k); err != nil {
				panic(fmt.Errorf("presence cannot parse dead key: %q", key))
			}
			k *= 63
			for i := int64(0); i < 64 && value > 0; i++ {
				on := value&1 == 1
				value >>= 1
				if !on {
					continue
				}
				seq := k + i
				dead[seq] = true
			}
		}
	}

	// Learn about all the pingers that reported and queue
	// events for those that weren't known to be alive and
	// are not reportedly dead either.
	alive := make(map[int64]bool)
	for i := range ping {
		for key, value := range ping[i].Alive {
			if _, err := fmt.Sscanf(key, "%x", &k); err != nil {
				panic(fmt.Errorf("presence cannot parse alive key: %q", key))
			}
			k *= 63
			for i := int64(0); i < 64 && value > 0; i++ {
				on := value&1 == 1
				value >>= 1
				if !on {
					continue
				}
				seq := k + i
				if dead[seq] {
					continue
				}
				alive[seq] = true
				if _, ok := w.beingKey[seq]; ok {
					continue
				}
				err := w.beings.Find(bson.D{{"seq", seq}}).One(&being)
				if err != nil {
					log.Printf("presence found unowned ping with seq=%d", seq)
				}
				w.beingKey[seq] = being.Key
				w.beingSeq[being.Key] = seq
				for _, ch := range w.watches[being.Key] {
					w.pending = append(w.pending, event{ch, being.Key, true})
				}
			}
		}
	}

	// Pingers that were known to be alive and haven't reported
	// in the last two slots are now considered dead. Dispatch
	// the respective events and forget their sequences.
	for seq, key := range w.beingKey {
		if !alive[seq] {
			delete(w.beingKey, seq)
			seq2, ok := w.beingSeq[key]
			if !ok || seq2 == seq || !alive[seq2] {
				delete(w.beingSeq, key)
				for _, ch := range w.watches[key] {
					w.pending = append(w.pending, event{ch, key, false})
				}
			}
		}
	}
	return nil
}

// A Pinger periodically reports that a specific key is alive, so that
// watchers interested on that fact can react appropriately.
type Pinger struct {
	base     *mgo.Collection
	pings    *mgo.Collection
	beingKey string
	beingSeq int64
	fieldKey string // hex(beingN / 63)
	fieldBit uint64 // 1 << (beingN%63)
	lastSlot int64
	delta    time.Duration
}

// NewPinger returns a new Pinger to report that key is alive. It
// will not start reporting until the Start method is called.
func NewPinger(base *mgo.Collection, key string) *Pinger {
	return &Pinger{base: base, pings: pingsC(base), beingKey: key}
}

// Start starts periodically reporting that the key the pinger
// is responsible for is alive.
func (p *Pinger) Start() error {
	// FIXME Error on double-starts
	var being beingInfo
	change := mgo.Change{
		Update:    bson.D{{"$inc", bson.D{{"seq", int64(1)}}}},
		Upsert:    true,
		ReturnNew: true,
	}
	seqs := seqsC(p.base)
	if _, err := seqs.FindId("beings").Apply(change, &being); err != nil {
		return err
	}
	p.beingSeq = being.Seq
	p.fieldKey = fmt.Sprintf("%x", p.beingSeq/63)
	p.fieldBit = 1 << uint64(p.beingSeq%63)
	beings := beingsC(p.base)
	if _, err := beings.UpsertId(p.beingKey, bson.D{{"$set", bson.D{{"seq", p.beingSeq}}}}); err != nil {
		return err
	}
	if p.delta == 0 {
		delta, err := clockDelta(p.base)
		if err != nil {
			return err
		}
		p.delta = delta
	}
	return p.ping()
}

func (p *Pinger) ping() error {
	p.lastSlot = timeSlot(time.Now(), p.delta)
	if _, err := p.pings.UpsertId(p.lastSlot, bson.D{{"$inc", bson.D{{"alive." + p.fieldKey, p.fieldBit}}}}); err != nil {
		return err
	}
	return nil
}

func (p *Pinger) Stop() error {
	return nil
}

func (p *Pinger) Kill() error {
	// FIXME Error on double-kills
	if _, err := p.pings.UpsertId(p.lastSlot, bson.D{{"$inc", bson.D{{"dead." + p.fieldKey, p.fieldBit}}}}); err != nil {
		return err
	}
	return nil
}

// clockDelta returns the approximate skew between
// the local clock and the database clock.
func clockDelta(c *mgo.Collection) (time.Duration, error) {
	var server struct {
		time.Time "retval"
	}
	for i := 0; i < 10; i++ {
		before := time.Now().UTC()
		err := c.Database.Run(bson.D{{"$eval", "function() { return new Date(); }"}}, &server)
		after := time.Now().UTC()
		if err != nil {
			return 0, err
		}
		delay := after.Sub(before)
		if delay > 5*time.Second {
			continue
		}
		return server.Sub(before), nil
	}
	return 0, fmt.Errorf("cannot synchronize clock with database server")
}

func timeSlot(now time.Time, delta time.Duration) int64 {
	fake := !fakeNow.IsZero()
	if fake {
		now = fakeNow
	}
	slot := now.Add(delta).Unix()
	slot -= slot % period
	if fake {
		slot += int64(fakeOffset) * period
	}
	return slot
}

var fakeNow time.Time
var fakeOffset int

func fakeTimeSlot(offset int) {
	if fakeNow.IsZero() {
		fakeNow = time.Now()
	}
	fakeOffset = offset
	log.Printf("Faking presence to time slot %d", offset)
}

func realTimeSlot() {
	fakeNow = time.Time{}
	fakeOffset = 0
	log.Printf("Not faking presence time. Real time slot in use.")
}

func seqsC(base *mgo.Collection) *mgo.Collection {
	return base.Database.C(base.Name + ".seqs")
}

func beingsC(base *mgo.Collection) *mgo.Collection {
	return base.Database.C(base.Name + ".beings")
}

func pingsC(base *mgo.Collection) *mgo.Collection {
	return base.Database.C(base.Name + ".pings")
}
