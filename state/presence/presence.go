// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// The presence package implements an interface for observing liveness
// of arbitrary keys (agents, processes, etc) on top of MongoDB.
// The design works by periodically updating the database so that
// watchers can tell an arbitrary key is alive.
package presence

import (
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"launchpad.net/tomb"
)

var logger = loggo.GetLogger("juju.state.presence")

type Presencer interface {
	AgentPresence() (bool, error)
	SetAgentPresence() (*Pinger, error)
	WaitAgentPresence(time.Duration) error
}

// docIDInt64 generates a globally unique id value
// where the environment uuid is prefixed to the
// given int64 localID.
func docIDInt64(envUUID string, localID int64) string {
	return envUUID + ":" + strconv.FormatInt(localID, 10)
}

// docIDStr generates a globally unique id value
// where the environment uuid is prefixed to the
// given string localID.
func docIDStr(envUUID string, localID string) string {
	return envUUID + ":" + localID
}

// The implementation works by assigning a unique sequence number to each
// pinger that is alive, and the pinger is then responsible for
// periodically updating the current time slot document with its
// sequence number so that watchers can tell it is alive.
//
// There is only one time slot document per time slot, per environment. The
// internal implementation of the time slot document is as follows:
//
// {
//   "_id":   <environ UUID>:<time slot>,
//   "slot": <slot>,
//   "env-uuid": <environ UUID>,
//   "alive": { hex(<pinger seq> / 63) : (1 << (<pinger seq> % 63) | <others>) },
//   "dead":  { hex(<pinger seq> / 63) : (1 << (<pinger seq> % 63) | <others>) },
// }
//
// All pingers that have their sequence number under "alive" and not
// under "dead" are currently alive. This design enables implementing
// a ping with a single update operation, a kill with another operation,
// and obtaining liveness data with a single query that returns two
// documents (the last two time slots).
//
// A new pinger sequence is obtained every time a pinger starts by atomically
// incrementing a counter in a document in a helper collection. There is only
// one such document per environment. That sequence number is then inserted
// into the beings collection to establish the mapping between pinger sequence
// and key.

// BUG(gn): The pings and beings collection currently grow without bound.

// A Watcher can watch any number of pinger keys for liveness changes.
type Watcher struct {
	envUUID string
	tomb    tomb.Tomb
	base    *mgo.Collection
	pings   *mgo.Collection
	beings  *mgo.Collection

	// delta is an approximate clock skew between the local system
	// clock and the database clock.
	delta time.Duration

	// beingKey and beingSeq are the pinger seq <=> key mappings.
	// Entries in these maps are considered alive.
	beingKey map[int64]string
	beingSeq map[string]int64

	// watches has the per-key observer channels from Watch/Unwatch.
	watches map[string][]chan<- Change

	// pending contains all the events to be dispatched to the watcher
	// channels. They're queued during processing and flushed at the
	// end to simplify the algorithm.
	pending []event

	// request is used to deliver requests from the public API into
	// the the gorotuine loop.
	request chan interface{}

	// syncDone contains pending done channels from sync requests.
	syncDone []chan bool

	// next will dispatch when it's time to sync the database
	// knowledge. It's maintained here so that ForceRefresh
	// can manipulate it to force a sync sooner.
	next <-chan time.Time
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

// NewWatcher returns a new Watcher.
func NewWatcher(base *mgo.Collection, envTag names.EnvironTag) *Watcher {
	w := &Watcher{
		envUUID:  envTag.Id(),
		base:     base,
		pings:    pingsC(base),
		beings:   beingsC(base),
		beingKey: make(map[int64]string),
		beingSeq: make(map[string]int64),
		watches:  make(map[string][]chan<- Change),
		request:  make(chan interface{}),
	}
	go func() {
		err := w.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("watcher loop failed: %v", err)
		}
		w.tomb.Kill(cause)
		w.tomb.Done()
	}()
	return w
}

// Stop stops all the watcher activities.
func (w *Watcher) Stop() error {
	w.tomb.Kill(nil)
	return errors.Trace(w.tomb.Wait())
}

// Dead returns a channel that is closed when the watcher has stopped.
func (w *Watcher) Dead() <-chan struct{} {
	return w.tomb.Dead()
}

// Err returns the error with which the watcher stopped.
// It returns nil if the watcher stopped cleanly, tomb.ErrStillAlive
// if the watcher is still running properly, or the respective error
// if the watcher is terminating or has terminated with an error.
func (w *Watcher) Err() error {
	return w.tomb.Err()
}

type reqWatch struct {
	key string
	ch  chan<- Change
}

type reqUnwatch struct {
	key string
	ch  chan<- Change
}

type reqSync struct {
	done chan bool
}

type reqAlive struct {
	key    string
	result chan bool
}

func (w *Watcher) sendReq(req interface{}) {
	select {
	case w.request <- req:
	case <-w.tomb.Dying():
	}
}

// Watch starts watching the liveness of key. An event will
// be sent onto ch to report the initial status for the key, and
// from then on a new event will be sent whenever a change is
// detected. Change values sent to the channel must be consumed,
// or the whole watcher will blocked.
func (w *Watcher) Watch(key string, ch chan<- Change) {
	w.sendReq(reqWatch{key, ch})
}

// Unwatch stops watching the liveness of key via ch.
func (w *Watcher) Unwatch(key string, ch chan<- Change) {
	w.sendReq(reqUnwatch{key, ch})
}

// StartSync forces the watcher to load new events from the database.
func (w *Watcher) StartSync() {
	w.sendReq(reqSync{nil})
}

// Sync forces the watcher to load new events from the database and blocks
// until all events have been dispatched.
func (w *Watcher) Sync() {
	done := make(chan bool)
	w.sendReq(reqSync{done})
	select {
	case <-done:
	case <-w.tomb.Dying():
	}
}

// Alive returns whether the key is currently considered alive by w,
// or an error in case the watcher is dying.
func (w *Watcher) Alive(key string) (bool, error) {
	result := make(chan bool, 1)
	w.sendReq(reqAlive{key, result})
	var alive bool
	select {
	case alive = <-result:
	case <-w.tomb.Dying():
		return false, errors.Errorf("cannot check liveness: watcher is dying")
	}
	return alive, nil
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
		return errors.Trace(err)
	}
	w.next = time.After(0)
	for {
		select {
		case <-w.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case <-w.next:
			w.next = time.After(time.Duration(period) * time.Second)
			syncDone := w.syncDone
			w.syncDone = nil
			if err := w.sync(); err != nil {
				return errors.Trace(err)
			}
			w.flush()
			for _, done := range syncDone {
				close(done)
			}
		case req := <-w.request:
			w.handle(req)
			w.flush()
		}
	}
}

// flush sends all pending events to their respective channels.
func (w *Watcher) flush() {
	// w.pending may get new requests as we handle other requests.
	for i := 0; i < len(w.pending); i++ {
		e := &w.pending[i]
		for e.ch != nil {
			select {
			case <-w.tomb.Dying():
				return
			case req := <-w.request:
				w.handle(req)
				continue
			case e.ch <- Change{e.key, e.alive}:
			}
			break
		}
	}
	w.pending = w.pending[:0]
}

// handle deals with requests delivered by the public API
// onto the background watcher goroutine.
func (w *Watcher) handle(req interface{}) {
	logger.Tracef("got request: %#v", req)
	switch r := req.(type) {
	case reqSync:
		w.next = time.After(0)
		if r.done != nil {
			w.syncDone = append(w.syncDone, r.done)
		}
	case reqWatch:
		for _, ch := range w.watches[r.key] {
			if ch == r.ch {
				panic("adding channel twice for same key")
			}
		}
		w.watches[r.key] = append(w.watches[r.key], r.ch)
		_, alive := w.beingSeq[r.key]
		w.pending = append(w.pending, event{r.ch, r.key, alive})
	case reqUnwatch:
		watches := w.watches[r.key]
		for i, ch := range watches {
			if ch == r.ch {
				watches[i] = watches[len(watches)-1]
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
		r.result <- alive
	default:
		panic(fmt.Errorf("unknown request: %T", req))
	}
}

type beingInfo struct {
	DocID   string `bson:"_id,omitempty"`
	Seq     int64  `bson:"seq,omitempty"`
	EnvUUID string `bson:"env-uuid,omitempty"`
	Key     string `bson:"key,omitempty"`
}

type pingInfo struct {
	DocID string           `bson:"_id,omitempty"`
	Slot  int64            `bson:"slot,omitempty"`
	Alive map[string]int64 `bson:",omitempty"`
	Dead  map[string]int64 `bson:",omitempty"`
}

func (w *Watcher) findAllBeings() (map[int64]beingInfo, error) {
	beings := make([]beingInfo, 0)
	session := w.beings.Database.Session.Copy()
	defer session.Close()
	beingsC := w.beings.With(session)

	err := beingsC.Find(bson.D{{"env-uuid", w.envUUID}}).All(&beings)
	if err != nil {
		return nil, err
	}
	beingInfos := make(map[int64]beingInfo, len(beings))
	for _, being := range beings {
		beingInfos[being.Seq] = being
	}
	return beingInfos, nil
}

// sync updates the watcher knowledge from the database, and
// queues events to observing channels. It fetches the last two time
// slots and compares the union of both to the in-memory state.
func (w *Watcher) sync() error {
	var allBeings map[int64]beingInfo
	if len(w.beingKey) == 0 {
		// The very first time we sync, we grab all ever-known beings,
		// so we don't have to look them up one-by-one
		var err error
		if allBeings, err = w.findAllBeings(); err != nil {
			return errors.Trace(err)
		}
	}
	s := timeSlot(time.Now(), w.delta)
	slot := docIDInt64(w.envUUID, s)
	previousSlot := docIDInt64(w.envUUID, s-period)
	session := w.pings.Database.Session.Copy()
	defer session.Close()
	pings := w.pings.With(session)
	var ping []pingInfo
	q := bson.D{{"$or", []pingInfo{{DocID: slot}, {DocID: previousSlot}}}}
	err := pings.Find(q).All(&ping)
	if err != nil && err == mgo.ErrNotFound {
		return errors.Trace(err)
	}

	// Learn about all enforced deaths.
	dead := make(map[int64]bool)
	for i := range ping {
		for key, value := range ping[i].Dead {
			k, err := strconv.ParseInt(key, 16, 64)
			if err != nil {
				err = errors.Annotatef(err, "presence cannot parse dead key: %q", key)
				panic(err)
			}
			k *= 63
			for i := int64(0); i < 63 && value > 0; i++ {
				on := value&1 == 1
				value >>= 1
				if !on {
					continue
				}
				seq := k + i
				dead[seq] = true
				logger.Tracef("found seq=%d dead", seq)
			}
		}
	}

	// Learn about all the pingers that reported and queue
	// events for those that weren't known to be alive and
	// are not reportedly dead either.
	beingsC := w.beings.With(session)
	alive := make(map[int64]bool)
	being := beingInfo{}
	for i := range ping {
		for key, value := range ping[i].Alive {
			k, err := strconv.ParseInt(key, 16, 64)
			if err != nil {
				err = errors.Annotatef(err, "presence cannot parse alive key: %q", key)
				panic(err)
			}
			k *= 63
			for i := int64(0); i < 63 && value > 0; i++ {
				on := value&1 == 1
				value >>= 1
				if !on {
					continue
				}
				seq := k + i
				alive[seq] = true
				if _, ok := w.beingKey[seq]; ok {
					continue
				}
				// Check if the being exists in the 'all' map,
				// otherwise do a single lookup in mongo
				var ok bool
				if being, ok = allBeings[seq]; !ok {
					err := beingsC.Find(bson.D{{"_id", docIDInt64(w.envUUID, seq)}}).One(&being)
					if err == mgo.ErrNotFound {
						logger.Tracef("found seq=%d unowned", seq)
						continue
					} else if err != nil {
						return errors.Trace(err)
					}
				}
				cur := w.beingSeq[being.Key]
				if cur < seq {
					delete(w.beingKey, cur)
				} else {
					// Current sequence is more recent.
					continue
				}
				w.beingKey[seq] = being.Key
				w.beingSeq[being.Key] = seq
				if cur > 0 || dead[seq] {
					continue
				}
				logger.Tracef("found seq=%d alive with key %q", seq, being.Key)
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
		if dead[seq] || !alive[seq] {
			delete(w.beingKey, seq)
			delete(w.beingSeq, key)
			for _, ch := range w.watches[key] {
				w.pending = append(w.pending, event{ch, key, false})
			}
		}
	}
	return nil
}

// Pinger periodically reports that a specific key is alive, so that
// watchers interested on that fact can react appropriately.
type Pinger struct {
	envUUID  string
	mu       sync.Mutex
	tomb     tomb.Tomb
	base     *mgo.Collection
	pings    *mgo.Collection
	started  bool
	beingKey string
	beingSeq int64
	fieldKey string // hex(beingKey / 63)
	fieldBit uint64 // 1 << (beingKey%63)
	lastSlot int64
	delta    time.Duration
}

// NewPinger returns a new Pinger to report that key is alive.
// It starts reporting after Start is called.
func NewPinger(base *mgo.Collection, envTag names.EnvironTag, key string) *Pinger {
	return &Pinger{
		base:     base,
		pings:    pingsC(base),
		beingKey: key,
		envUUID:  envTag.Id(),
	}
}

// Start starts periodically reporting that p's key is alive.
func (p *Pinger) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return errors.Errorf("pinger already started")
	}
	p.tomb = tomb.Tomb{}
	if err := p.prepare(); err != nil {
		return errors.Trace(err)
	}
	logger.Tracef("starting pinger for %q with seq=%d", p.beingKey, p.beingSeq)
	if err := p.ping(); err != nil {
		return errors.Trace(err)
	}
	p.started = true
	go func() {
		err := p.loop()
		cause := errors.Cause(err)
		// tomb expects ErrDying or ErrStillAlive as
		// exact values, so we need to log and unwrap
		// the error first.
		if err != nil && cause != tomb.ErrDying {
			logger.Infof("pinger loop failed: %v", err)
		}
		p.tomb.Kill(cause)
		p.tomb.Done()
	}()
	return nil
}

// Stop stops p's periodical ping.
// Watchers will not notice p has stopped pinging until the
// previous ping times out.
func (p *Pinger) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		logger.Tracef("stopping pinger for %q with seq=%d", p.beingKey, p.beingSeq)
	}
	p.tomb.Kill(nil)
	err := p.tomb.Wait()
	// TODO ping one more time to guarantee a late timeout.
	p.started = false
	return errors.Trace(err)

}

// Kill stops p's periodical ping and immediately reports that it is dead.
func (p *Pinger) Kill() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		logger.Tracef("killing pinger for %q (was started)", p.beingKey)
		return p.killStarted()
	}
	logger.Tracef("killing pinger for %q (was stopped)", p.beingKey)
	return p.killStopped()
}

// killStarted kills the pinger while it is running, by first
// stopping it and then recording in the last pinged slot that
// the pinger was killed.
func (p *Pinger) killStarted() error {
	p.tomb.Kill(nil)
	killErr := p.tomb.Wait()
	p.started = false

	slot := p.lastSlot
	udoc := bson.D{
		{"$set", bson.D{{"slot", slot}}},
		{"$inc", bson.D{{"dead." + p.fieldKey, p.fieldBit}}}}
	session := p.pings.Database.Session.Copy()
	defer session.Close()
	pings := p.pings.With(session)
	if _, err := pings.UpsertId(docIDInt64(p.envUUID, slot), udoc); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(killErr)
}

// killStopped kills the pinger while it is not running, by
// first allocating a new sequence, and then atomically recording
// the new sequence both as alive and dead at once.
func (p *Pinger) killStopped() error {
	if err := p.prepare(); err != nil {
		return err
	}
	slot := timeSlot(time.Now(), p.delta)
	udoc := bson.D{
		{"$set", bson.D{{"slot", slot}}},
		{"$inc", bson.D{
			{"dead." + p.fieldKey, p.fieldBit},
			{"alive." + p.fieldKey, p.fieldBit},
		}}}
	session := p.pings.Database.Session.Copy()
	defer session.Close()
	pings := p.pings.With(session)
	_, err := pings.UpsertId(docIDInt64(p.envUUID, slot), udoc)
	return errors.Trace(err)
}

// loop is the main pinger loop that runs while it is
// in started state.
func (p *Pinger) loop() error {
	for {
		select {
		case <-p.tomb.Dying():
			return errors.Trace(tomb.ErrDying)
		case <-time.After(time.Duration(float64(period+1)*0.75) * time.Second):
			if err := p.ping(); err != nil {
				return errors.Trace(err)
			}
		}
	}
}

// prepare allocates a new unique sequence for the
// pinger key and prepares the pinger to use it.
func (p *Pinger) prepare() error {
	change := mgo.Change{
		Update:    bson.D{{"$inc", bson.D{{"seq", int64(1)}}}},
		Upsert:    true,
		ReturnNew: true,
	}
	session := p.base.Database.Session.Copy()
	defer session.Close()
	base := p.base.With(session)
	seqs := seqsC(base)
	var seq struct{ Seq int64 }
	seqID := docIDStr(p.envUUID, "beings")
	if _, err := seqs.FindId(seqID).Apply(change, &seq); err != nil {
		return errors.Trace(err)
	}
	p.beingSeq = seq.Seq
	p.fieldKey = fmt.Sprintf("%x", p.beingSeq/63)
	p.fieldBit = 1 << uint64(p.beingSeq%63)
	p.lastSlot = 0
	beings := beingsC(base)
	return errors.Trace(beings.Insert(
		beingInfo{
			DocID:   docIDInt64(p.envUUID, p.beingSeq),
			Seq:     p.beingSeq,
			EnvUUID: p.envUUID,
			Key:     p.beingKey,
		},
	))
}

// ping records updates the current time slot with the
// sequence in use by the pinger.
func (p *Pinger) ping() (err error) {
	logger.Tracef("pinging %q with seq=%d", p.beingKey, p.beingSeq)
	defer func() {
		// If the session is killed from underneath us, it panics when we
		// try to copy it, so deal with that here.
		if v := recover(); v != nil {
			if v == "Session already closed" {
				return
			}
			err = fmt.Errorf("%v", v)
		}
	}()
	session := p.pings.Database.Session.Copy()
	defer session.Close()
	if p.delta == 0 {
		base := p.base.With(session)
		delta, err := clockDelta(base)
		if err != nil {
			return errors.Trace(err)
		}
		p.delta = delta
	}
	slot := timeSlot(time.Now(), p.delta)
	if slot == p.lastSlot {
		// Never, ever, ping the same slot twice.
		// The increment below would corrupt the slot.
		return nil
	}
	p.lastSlot = slot
	pings := p.pings.With(session)
	_, err = pings.UpsertId(
		docIDInt64(p.envUUID, slot),
		bson.D{
			{"$set", bson.D{{"slot", slot}}},
			{"$inc", bson.D{{"alive." + p.fieldKey, p.fieldBit}}},
		})
	return errors.Trace(err)
}

// clockDelta returns the approximate skew between
// the local clock and the database clock.
func clockDelta(c *mgo.Collection) (time.Duration, error) {
	var server struct {
		time.Time `bson:"retval"`
	}
	var isMaster struct {
		LocalTime time.Time `bson:"localTime"`
	}
	var after time.Time
	var before time.Time
	var serverDelay time.Duration
	supportsMasterLocalTime := true
	session := c.Database.Session.Copy()
	defer session.Close()
	db := c.Database.With(session)
	for i := 0; i < 10; i++ {
		if supportsMasterLocalTime {
			// Try isMaster.localTime, which is present since MongoDB 2.2
			// and does not require admin privileges.
			before = time.Now()
			err := db.Run("isMaster", &isMaster)
			after = time.Now()
			if err != nil {
				return 0, errors.Trace(err)
			}
			if isMaster.LocalTime.IsZero() {
				supportsMasterLocalTime = false
				continue
			} else {
				serverDelay = isMaster.LocalTime.Sub(before)
			}
		} else {
			// If MongoDB doesn't have localTime as part of
			// isMaster result, it means that the server is likely
			// a MongoDB older than 2.2.
			//
			// Fallback to 'eval' works fine on versions older than
			// 2.4 where it does not require admin privileges.
			//
			// NOTE: 'eval' takes a global write lock unless you
			// specify 'nolock' (which we are not doing below, for
			// no apparent reason), so it is quite likely that the
			// eval could take a relatively long time to acquire
			// the lock and thus cause a retry on the callDelay
			// check below on a busy server.
			before = time.Now()
			err := db.Run(bson.D{{"$eval", "function() { return new Date(); }"}}, &server)
			after = time.Now()
			if err != nil {
				return 0, errors.Trace(err)
			}
			serverDelay = server.Sub(before)
		}
		// If the call to the server takes longer than a few seconds we
		// retry it a couple more times before giving up. It is unclear
		// why the retry would help at all here.
		//
		// If the server takes longer than the specified amount of time
		// on every single try, then we simply give up.
		callDelay := after.Sub(before)
		if callDelay > 5*time.Second {
			continue
		}
		return serverDelay, nil
	}
	return 0, errors.Errorf("cannot synchronize clock with database server")
}

// timeSlot returns the current time slot, in seconds since the
// epoch, for the provided now time. The delta skew is applied
// to the now time to improve the synchronization with a
// centrally agreed time.
//
// The result of this method may be manipulated for test purposes
// by fakeTimeSlot and realTimeSlot.
func timeSlot(now time.Time, delta time.Duration) int64 {
	fakeMutex.Lock()
	fake := !fakeNow.IsZero()
	if fake {
		now = fakeNow
	}
	slot := now.Add(delta).Unix()
	slot -= slot % period
	if fake {
		slot += int64(fakeOffset) * period
	}
	fakeMutex.Unlock()
	return slot
}

var (
	fakeMutex  sync.Mutex // protects fakeOffset, fakeNow
	fakeNow    time.Time
	fakeOffset int
)

// fakeTimeSlot hardcodes the slot time returned by the timeSlot
// function for testing purposes. The offset parameter is the slot
// position to return: offsets +1 and -1 are +period and -period
// seconds from slot 0, respectively.
func fakeTimeSlot(offset int) {
	fakeMutex.Lock()
	if fakeNow.IsZero() {
		fakeNow = time.Now()
	}
	fakeOffset = offset
	fakeMutex.Unlock()
	logger.Infof("faking presence to time slot %d", offset)
}

// realTimeSlot disables the hardcoding introduced by fakeTimeSlot.
func realTimeSlot() {
	fakeMutex.Lock()
	fakeNow = time.Time{}
	fakeOffset = 0
	fakeMutex.Unlock()
	logger.Infof("not faking presence time. Real time slot in use.")
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
