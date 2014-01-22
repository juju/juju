type worker struct {
	wg sync.WaitGroup

	// When something changes that might might affect
	// the peer group membership, it sends a function
	// on notifyc that is run inside the main worker
	// goroutine to mutate the state. It reports whether
	// the state has actually changed.
	notifyc chan func() (bool, error)
}

// machine represents a machine in State.
type machine struct {
	id        string
	wantsVote bool
	hostPort  string
	mongoPort string

	worker *worker
}

func (w *worker) loop() error {
	idsWatcher := w.st.WatchStateServerIds()
	defer idsWatcher.Stop()

	notifyc := make(chan func() (bool, error))
	info := &peerGroupInfo{
		machines: make(map[string] *machine),
	}
	var wg sync.WaitGroup
	for {
		select {
		case ids, ok := <-idsWatcher.Changes():
			if !ok {
				...
			}
			// Stop machines that are no longer state servers.
			for _, m := range machines {
				if !inStrings(m.id, ids) {
					m.stop()
				}
			}
			// Start machines with no watcher
			for _, id := range ids {
				if _, ok := machines[id]; ok {
					continue
				}
				stm, err := w.st.Machine(id)
				if err != nil {
					if errors.IsNotFound(err) {
						continue
					}
				}
				info.machines[id] = w.newMachine(stm)
			}
		case f := <-notifyc:
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
			w.setPeerGroup(members)
		}
						
	}
}

perhaps rename machine to machineInfo?

func (w *worker) newMachine(stm *state.Machine) *machine {
	m := &machine{
		worker: w,
		id: stm.Id(),
		wantsVote: stm.WantsVote(),
		hostPort: w.mongoHostPort(stm.Addresses()),

		stm *state.Machine
	}
	go func() {
		if err := m.loop(stm.Watch()); err != nil {
			w.notifyError(err)
		}
	}()
	return m
}

func (w *worker) notifyError(err error) {
	w.notify(func() (bool, error) {
		return false, err
	})
}

func (w *worker) notify(func() (bool, error)) bool {
	select {
	case w.notifyc <- f:
		return true
	case <-w.tomb.Dying():
		return false
	}
}

func (m *

XXX should we kill everything if a machine isn't found?

func (m *machine) loop(watcher *state.NotifyWatcher) error {
	defer m.worker.wg.Done()
	for {
	case <-watcher.Changes():
		if err := m.stm.Refresh(); err != nil {
			return err
		}

	case <-w.tomb.Dying():
}

func (w *worker) peerGroupSetter(memberc <-chan []replicaset.Member) {
	upToDate := true
	for {
		var pollc <-chan time.Time
		if !ok {
			pollc = time.After(pollInterval)
		}
		select {
		case <-w.tomb.Dying():
			w.wg.Done()
			return
		case members := <-memberc:
			upToDate = false
		case <-pollc:
			if err := replicset.Set(session, members); err != nil {
				log.Errorf("cannot set replicaset: %v", err)
			} else {
				upToDate = true
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