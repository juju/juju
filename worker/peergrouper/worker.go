type worker struct {
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
				info.machines[id] = newMachine(id, w.wg, stm, notifyc)
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

func (w *worker) peerGroupSetter(memberc <-chan []replicaset.Member) {
	ok := true
	for {
		var pollc <-chan time.Time
		if !ok {
			pollc = time.After(pollInterval)
		}
		select {
		case <-w.tomb.Dying():
			w.wg.Done()
		case members := <-memberc:
			ok = false
		case <-pollc:
			if err := replicset.Set(session, members); err != nil {
				log.Errorf("cannot set replicaset: %v", err)
			} else {
				ok = true
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