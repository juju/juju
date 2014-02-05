// +build ignore

type notifyFunc func() (bool, error)

type worker struct {
	wg sync.WaitGroup

	// When something changes that might might affect
	// the peer group membership, it sends a function
	// on notifyCh that is run inside the main worker
	// goroutine to mutate the state. It reports whether
	// the state has actually changed.
	notifyCh chan notifyFunc

	mongoPort int

	setPeerGroupCh chan []replicaset.Member

	machines 
}


func New(st *state.State) worker.Worker {
	w := &worker{
		notifyCh: chan notifyFunc,
		setPeerGroupCh: make(chan []replicaset.Member),
	}
	w.wg.Add(1)
	go w.peerGroupSetter()
	go func() {
		defer w.tomb.Done()
		defer w.wg.Wait()
		if err := worker.loop(); err != nil {
			tomb.Kill(err)
		}
	}()
	return w
}

// machine represents a machine in State.
type machine struct {
	id        string
	wantsVote bool
	hostPort  string

	worker *worker
	stm *state.Machine
	machineWatcher *state.NotifyWatcher
}

issues:
what do we need to watch?

	wantsVote:
		WatchMachine machine.WantsVote
	hostPort:
		WatchMachine machine.Addresses
		port comes from environ config and doesn't change

what do we need to change?

	set replicaset
	Machine.HasVote

which do we treat as authoritative in for deciding
whether a machine wants the vote, StateServerInfo
or the machine itself?

The former has a better guarantee, so we should
use that. But... what if the machine itself has gone,
although it's still configured as a state server?

Q. how do we update the HasVote status of each
machine?

the real question is:
do we set the voting status of a machine before
or after we actually try to set it in the peer group?

if we set it afterwards, then a machine might be
decomissioned inappropriately.

if the machine's isn't configured with hasVote
before calling setPeerGroup,
the state front end is free to set the machine
to dying, which means that the machine
could go away even though the machine
is a voter.



we *could* set the voting status of all new
voting machines before calling setPeerGroup,
then set reset the voting status of all machines
that have become non-voting afterwards.



status, which could mean that the machine is
set to dying kills itself even though it's one
of the voting machines.


func (w *worker) loop() error {
	idsWatcher := w.st.WatchStateServerInfo()
	defer idsWatcher.Stop()

	notifyCh := make(notifyFunc)
	info := &peerGroupInfo{
		machines: w.machines,
	}
	timer := time.NewTimer(0)
	timer.Stop()

	var desiredVoting map[*machine] bool
	var desiredMembers []replicaset.Member
	for {
		select {
		case f := <-w.notifyCh:
			// Update our current view of the state of affairs.
			changed, err := f()
			if err != nil {
				return err
			}
			if !changed {
				break
			}
			members, voting, err := desiredPeerGroup(info)
			if err != nil {
				logger.Errorf("cannot compute desired peer group: %v", err)
				continue
			}
			if members == nil {
				timer.Stop()
				break
			}
			desiredMembers = members
			desiredVoting = voting
			// Try to change the replica set immediately.
			timer.Reset(0)
		case <-timer.C:
			if err := setReplicaSet(session, desiredMembers, desiredVoting); err != nil {
				if _, isReplicaSetError := err.(*replicasetError); !isReplicaSetError {
					return err
				}
				logger.Errorf("cannot set replicaset: %v", err)
				timer.Reset(retryInterval)
			}
			logger.Infof("successfully changed replica set to %#v", desiredMembers)
		}
	}
}

type replicaSetError struct {
	error
}

func setReplicaSet(session *mgo.Session, members []replicaset.Member, voting map[*machine] bool) error {
	// We cannot change the HasVote flag of a machine in state at exactly
	// the same moment as changing its voting status in the replica set.
	//
	// Thus we need to be careful that a machine which is actually a voting
	// member is not seen to not have a vote, because otherwise
	// there is nothing to prevent the machine being removed.
	//
	// To avoid this happening, we make sure when we call SetReplicaSet,
	// that the voting status of machines is the union of both old
	// and new voting machines - that is the set of HasVote machines
	// is a superset of all the actual voting machines.
	//
	// Only after the call has taken place do we reset the voting status
	// of the machines that have lost their vote.
	//
	// If there's a crash, the voting status may not reflect the
	// actual voting status for a while, but when things come
	// back on line, it will be sorted out, as desiredReplicaSet
	// will return the actual voting status.

	var added, removed []*machine
	for m, hasVote := range voting {
		switch {
		case hasVote && !m.stm.HasVote():
			added = append(added, m)
		case !hasVote && m.stm.HasVote():
			removed = append(removed, m)
		}
	}
	if err := setHasVote(added, true); err != nil {
		return err
	}
	if err := replicaset.Set(session, members); err != nil {
		// We've failed to set the replica set, so revert back
		// to the previous settings.
		if err1 := setHasVote(added, false); err1 != nil {
			log.Errorf("cannot revert machine voting after failure to change replica set")
		}
		return &replicaSetError{err}
	}
	if err := setHasVote(removed, false); err != nil {
		return err
	}
	return nil
}

func setHasVote(ms []*machine, hasVote bool) error {
	for _, m := range ms {
		if err := m.SetHasVote(hasVote); err != nil {
			return err
		}
	}
}

func (w *worker) watchStateServerInfo() *serverInfoWatcher {
	infow := &serverInfoWatcher{
		worker: w,
		watcher: w.st.WatchStateServerInfo(),
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := infow.loop(); err != nil {
			w.tomb.Kill(err)
		}
	}()
}

type serverInfoWatcher struct {
	worker *worker
	watcher *state.NotifyWatcher
}

func (infow *serverInfoWatcher) loop() error {
	for {
		select {
		case _, ok := <-infow.watcher.Changes():
			if !ok {
				return watcher.MustErr(idsWatcher)
			}
			infow.worker.notify(infow.updateMachines)
		case <-infow.worker.tomb.Dying():
			return tomb.ErrDying
		}
	}
}

// updateMachines is a notifyFunc that updates the current
// machines when the state server info has changed.
func (infow *serverInfoWatcher) updateMachines() (bool, error) {
	info, err := infow.worker.st.StateServerInfo()
	if err != nil {
		return false, err
	}
	// Stop machine goroutines that no longer correspond to state server
	// machines.
	for _, m := range infow.w.machines {
		if !inStrings(m.id, info.MachineIds) {
			m.stop()
			delete(infow.worker.machines, m.id)
		}
	}
	// Start machines with no watcher
	for _, id := range ids {
		if _, ok := infow.worker.machines[id]; ok {
			continue
		}
		stm, err := w.st.Machine(id)
		if err != nil {
			if errors.IsNotFound(err) {
				// If the machine isn't found, it must have been
				// removed and will soon enough be removed
				// from the state server list. This will probably
				// never happen, but we'll code defensively anyway. 
				continue
			}
		}
		infow.worker.machines[id] = w.newMachine(stm)
	}
}

func (w *worker) setPeerGroup(members []replicaset.Member) {
	select {
	case w.setPeerGroupCh <- members:
	case <-w.tomb.Dying():
	}
}

func (w *worker) notifyError(err error) {
	w.notify(func() (bool, error) {
		return false, err
	})
}

func (w *worker) notify(f notifyFunc) bool {
	select {
	case w.notifyCh <- f:
		return true
	case <-w.tomb.Dying():
		return false
	}
}

func (w *worker) newMachine(stm *state.Machine) *machine {
	m := &machine{
		worker: w,
		id: stm.Id(),
		stm: stm,
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		if err := m.loop(); err != nil {
			w.notifyError(err)
		}
	}()
	return m
}

func (m *machine) loop(mwatcher *state.NotifyWatcher) error {
	defer m.worker.wg.Done()
	m.machineWatcher := m.stm.Watch()
	for {
	case _, ok := <-m.machineWatcher.Changes():
		if !ok {
			return machineWatcher.Err()
		}
		m.worker.notify(m.refresh)
	case <-w.tomb.Dying():
	}
}

func (m *machine) stop() {
	m.machineWatcher.Stop()
}

func (m *machine) refresh() (bool, error) {
	if err := m.stm.Refresh(); err != nil {
		if errors.IsNotFoundError(err) {
			// We want to be robust when the machine
			// state is out of date with respect to the
			// state server info, so if the machine
			// has been removed, just assume that
			// no change has happened - the machine
			// loop will be stopped very soon anyway.
			return false, nil
		}
		return false, err
	}
	changed := false
	if wantsVote := m.stm.WantsVote(); wantsVote != m.wantsVote {
		m.wantVote = wantsVote
		changed = true
	}
	hostPort := instance.SelectInternalAddress(m.Addresses(), false)
	if hostPort != "" {
		host = joinHostPort(host, m.worker.mongoPort)
	}
	if hostPort != m.hostPort {
		m.hostPort = hostPort
		changed = true
	}
	return changed, nil
}

func joinHostPort(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

func (w *worker) peerGroupSetter() {
	defer w.wg.Done()
	timer := time.NewTimer(0)
	timer.Stop()
	for {
		select {
		case <-w.tomb.Dying():
			return
		case members, ok := <-w.setPeerGroupCh:
			if !ok {
				return
			}
			timer.Reset(0)
		case <-timer.C:
			if err := replicaset.Set(session, members); err != nil {
				log.Errorf("cannot set replicaset: %v", err)
				timer.Reset(retryInterval)
			}
		}
	}
}

func inStrings(ss []string, t string) bool {
	for _, s := range ss {
		if s == t {
			return true
		}
	}
	return false
}


// getPeerGroupInfo collates current session information about the
// mongo peer group with information from state machines.
func (w *worker) peerGroupInfo() (*peerGroupInfo, error) {
	session := w.st.MongoSession()
	info := &peerGroupInfo{}
	var err error
	info.statuses, err = replicaset.CurrentStatus(session)
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set status: %v", err)
	}
	info.members, err = replicaset.CurrentMembers(session)
	if err != nil {
		return nil, fmt.Errorf("cannot get replica set members: %v", err)
	}
	for _, m := range ms {
		info.machines = append(info.machines, &machine{
			id:        m.Id(),
			candidate: m.IsCandidate(),
			host: instance.SelectInternalAddress(m.Addresses(), false),
		})
	}
	return info, nil
}

