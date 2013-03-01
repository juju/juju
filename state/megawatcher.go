
type StateWatcher struct {
	commonWatcher
	out chan watcher.Change
}

func newStateWatcher(st *State) *StateWatcher {
	w := &StateWatcher{
		commonWatcher: commonWatcher{st: st},
		out: make(chan watcher.Change),
	}
	go func() {
		defer w.tomb.Done()
		defer close(w.out)
		w.tomb.Kill(w.loop())
	}()
	return w
}

// EntityInfo is implemented by all entity Info types.
type EntityInfo interface {
	EntityId() interface{}
	EntityKind() string
}

// MachineInfo holds the information about a Machine
// that is watched by StateWatcher.
type MachineInfo struct {
	Id           string `bson:"_id"`
	InstanceId string
}

func (i *MachineInfo) EntityId() interface{} { return i.Id }
func (i *MachineInfo) EntityKind() string { return "machine" }

type ServiceInfo struct {
	Name          string `bson:"_id"`
	Exposed       bool
}

func (i *ServiceInfo) EntityId() interface{} { return i.Name }
func (i *ServiceInfo) EntityKind() string { return "service" }

type UnitInfo struct {
	Name          string `bson:"_id"`
	Service        string
}

func (i *UnitInfo) EntityId() interface{} { return i.Name }
func (i *UnitInfo) EntityKind() string { return "service" }

func (i *ServiceInfo) EntityId() interface{} { return i.Name }
func (i *ServiceInfo) EntityKind() string { return "service" }


// infoEntityId returns the entity id of the given entity document.
func infoEntityId(st *state.State, info EntityInfo) entityId {
	return entityId{
		collection: docCollection(st, doc).Name,
		id: info.EntityId(),
	}
}

// infoCollection returns the collection that holds the
// given kind of entity info. This isn't defined on
// EntityInfo because we don't want to require all
// entities to hold a pointer to the state.
func infoCollection(st *State, i EntityInfo) *mgo.Collection {
	switch i := i.(type) {
	case *MachineInfo:
		return st.machines
	}
	panic("unknown entity type")
}

// entityId holds the mongo identifier of an entity.
type entityId struct {
	collection string
	id interface{}
}

// entityEntry holds an entry in the linked list
// of all entities known to a StateWatcher.
type entityEntry struct {
	// The revno holds the local idea of the latest change.
	// It is not the same as the transaction revno so that
	// we can unconditionally move a newly fetched entity
	// to the front of the list without worrying if the revno
	// has changed since the watcher reported it.
	revno int64
	removed bool
	info EntityInfo
}

// allInfo holds a list of all entities known
// to a StateWatcher.
type allInfo struct {
	st *state.State
	newInfo map[string] func() EntityInfo
	latestRevno int64
	entities map[entityId] *list.Element
	list *list.List
}

// add adds a new entity to the list.
func (a *allInfo) add(doc EntityInfo) {
	a.latestRevno++
	info := &entityEntry{
		doc: doc,
		revno: a.latestRevno,
	}
	a.entities[docEntityId(a.st, doc)] = a.list.PushFront(info)
}

// delete deletes an entity from the list.
func (a *allInfo) delete(id entityId) {
	if elem := a.entities[id]; elem != nil {
		if !elem.Value.(*entityEntry).removed {
			panic("deleting entry that has not been marked as removed")
		}
		delete(a.entities, id)
		a.list.Remove(elem)
	}
}

// update updates information on the entity
// with the given id by retrieving its information
// from mongo.
func (a *allInfo) update(id entityId) error {
	info := a.newInfo[id.collection]()
	collection := infoCollection(a.st, info)
	// TODO(rog) investigate ways that this can be made more efficient.
	if err := collection.FindId(info.EntityId()).One(info); err != nil {
		if IsNotFound(err) {
			// The document has been removed since the change notification arrived.
			if elem := a.entities[id]; elem != nil {
				elem.Value.(*entityEntry).removed = true
			}
			return nil
		}
		return fmt.Errorf("cannot get %v from %s: %v", id.id, collection.Name, err)
	}
	if elem := a.entities[id]; elem != nil {
		entry := elem.Value.(*entityEntry)
		// Nothing has changed, so change nothing.
		// TODO(rog) do the comparison more efficiently.
		if reflect.DeepEqual(info, entry.info) {
			return nil
		}
		// We already know about the entity; update its doc.
		a.latestRevno++
		entry.revno = a.latestRevno
		entry.info = info
		a.list.MoveToFront(elem)
	} else {
		a.add(info)
	}
}

// getAll retrieves information about all known
// entities from mongo.
func (a *allInfo) getAll() error {
	var mdocs []machineDoc
	err := w.st.machines.Find(nil).All(&mdocs)
	if err != nil {
		return fmt.Errorf("cannot get all machines: %v", err)
	}
	for i := range mdocs {
		all.add(&mdocs[i])
	}
}

var kindOrder = []string{
	"service",
	"relation",
	"machine",
	"unit",
}

func (a *allInfo) changesSince(revno int64) ([]Delta, int64) {
	// Extract all deltas into categorised slices, then
	// build up an overall slice that sends creates before
	// deletes, and orders parents before children
	// on creation, and children before parents on deletion
	// (see kindOrder above).
	e := a.list.Front()
	for ; e != nil; e = e.Next() {
		entry := e.Value.(*entityEntry)
		if entry.revno <= revno {
			break
		}
	}
	if e != nil {
		// We've found an element that we've already seen.
		e = e.Prev()
	} else {
		// We haven't seen any elements, so we want all of them.
		e = e.list.Back()
	}
	if e == nil {
		// Common case: nothing new to see - let's be efficient.
		return nil, revno
	}
	deltas := map[bool]{
		false: make(map[string][]Delta),
		true: make(map[string][]Delta),
	}
	n := 0
	// Iterate from oldest to newest.
	for ; e != nil; e = e.Prev() {
		entry := e.Value.(*entityEntry)
		if entry.revno <= revno {
			break
		}
		m := deltas[entry.removed]
		kind := entry.info.EntityKind()
		m[kind] = append(m[kind], Delta{
			Removed: entry.removed,
			Entity: entry.info,
		})
		n++
	}
	changes := make([]Delta, 0, n)
	// Changes in parent-to-child order
	for _, kind := range kindOrder {
		changes = append(changes, deltas[false][kind])
	}
	// Removals in child-to-parent order.
	for i := len(kindOrder)-1; i >= 0; i-- {
		changes = append(changes, deltas[true][kind])
	}
	return changes, a.list.Front().Value.(*entityEntry).revno
}

func (w *StateWatcher) loop() error {
	all := &allInfo{
		st: w.st,
		entities: make(map[entityId] *list.Element),
		newDoc: map[string] func() entityDoc {
			w.st.machines.Name: func() entityDoc {return new(machineDoc)},
			// etc
		},
		list: list.New(),
	}
	if err := all.getAll(); err != nil {
		return err
	}
	// TODO(rog) make this a collection of outstanding requests.
	var currentReq *getRequest

	in := make(chan watcher.Change)
	w.st.watcher.WatchCollection(w.st.machines.Name, in)
	defer w.st.watcher.UnwatchCollection(w.st.machines.Name, in)
	w.st.watcher.WatchCollection(w.st.services.Name, in)
	defer w.st.watcher.UnwatchCollection(w.st.services.Name, in)
	w.st.watcher.WatchCollection(w.st.units.Name, in)
	defer w.st.watcher.UnwatchCollection(w.st.units.Name, in)
	w.st.watcher.WatchCollection(w.st.relations.Name, in)
	defer w.st.watcher.UnwatchCollection(w.st.relations.Name, in)
	for {
		select {
		case <-w.tomb.Dying():
			return tomb.ErrDying
		case ch := <-in:
			if err := all.update(entityId{ch.C, ch.Id}); err != nil {
				return err
			}
		case req := <-w.request:
			if currentReq != nil {
				// TODO(rog) relax this
				panic("cannot have two outstanding get requests")
			}
			currentReq = req
		}
		// Satisfy any request that can be satisfied.
		if currentReq == nil {
			continue
		}
		changes, revno := all.changesSince(currentReq.revno)
		if len(changes) == nil {
			continue
		}
		currentReq.revno = revno
		currentReq.changes = changes
		currentReq.reply <- true
		currentReq = nil
	}
	panic("unreachable")
}

type getRequest struct {
	// On send, revno holds the requested revision number;
	// On reply, revno will hold the revision number
	// of the latest delta.
	revno int64
	// On reply, changes will hold all changes newer
	// then the requested revision number.
	changes []Delta
	// reply receives a message when deltas are ready.
	reply chan bool
}

// Get retrieves all changes that have happened since
// the given revision number, blocking until there
// are some changes available. It also returns the
// revision number of the latest change.
func (w *StateWatcher) Get(revno int64) ([]Delta, int64, error) {
	// TODO allow several concurrent Gets on the
	// same allInfo structure.
	req := getRequest{
		revno: revno,
		reply: make(chan bool),
	}
	w.request <- req
	if ok := <-req.reply; !ok {
		// TODO better error
		return 0, nil, fmt.Errorf("state watcher was stopped")
	}
	return w.revno, w.changes, nil
}

type Delta struct {
	Delete bool
	Entity EntityInfo
}

func (d *Delta) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(d.Entity)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteByte('[')
	c := "change"
	if d.Remove {
		c = "remove"
	}
	fmt.Fprintf(&buf, "[%q,%q,", d.Entity.EntityKind(), c)
	buf.Write(b)
	buf.WriteByte(']')
	return buf.Bytes(), nil
}

//func (d *Delta) UnmarshalJSON(b []byte) error {
//	var x []interface{}
//	if err := json.Unmarshal(b, &x); err != nil {
//		return err
//	}
//	if len(x) != 3 {
//		return fmt.Errorf("bad delta JSON %q", b)
//	}
//	switch x[0] {
//	case "change":
//	case "delete":
//		d.Delete = true
//	default:
//		return fmt.Errorf("bad delta JSON %q", b)
//	}
//	switch x[1] {
//	case "machine":		// TODO etc
//		d.Kind = x[1].(string)
//	default:
//		return fmt.Errorf("bad delta JSON %q", b)
//	}
//	
//}
