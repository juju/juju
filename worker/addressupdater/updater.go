package addressupdater

import (
	"fmt"
	"time"

	"launchpad.net/loggo"

	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/watcher"
)

var logger = loggo.GetLogger("juju.worker.addressupdater")

var (
	longPoll  = 10 * time.Second
	shortPoll = 500 * time.Millisecond
)

//func NewAddressPublisher() worker.Worker {
//	p := &updater{
//		st:
//	}
//	// wait for environment
//	go func() {
//		defer p.tomb.Done()
//		p.tomb.Kill(p.loop())
//	}()
//}

//type updater struct {
//	st   *state.State
//	tomb tomb.Tomb
//
//	mu      sync.Mutex
//	environ environs.Environ
//}

type machine interface {
	Id() string
	Addresses() []instance.Address
	InstanceId() (instance.Id, error)
	SetAddresses([]instance.Address) error
	Jobs() []state.MachineJob
	String() string
	Refresh() error
	Life() state.Life
}

type machineContext interface {
	killAll(err error)
	addresses(id instance.Id) ([]instance.Address, error)
	dying() <-chan struct{}
}

type machineAddress struct {
	machine   machine
	addresses []instance.Address
}

var _ machine = (*state.Machine)(nil)

type machinesWatcher interface {
	Changes() <-chan []string
	Err() error
	Stop() error
}

type updaterContext interface {
	newMachineContext() machineContext
	getMachine(id string) (machine, error)
	dying() <-chan struct{}
}

type updater struct {
	context     updaterContext
	machines    map[string]chan struct{}
	machineDead chan machine
}

// watchMachinesLoop watches for changes provided by the given
// machinesWatcher and starts machine goroutines to deal
// with them, using the provided newMachineContext
// function to create the appropriate context for each new machine id.
func watchMachinesLoop(context updaterContext, w machinesWatcher) (err error) {
	p := &updater{
		context:     context,
		machines:    make(map[string]chan struct{}),
		machineDead: make(chan machine),
	}
	defer func() {
		if stopErr := w.Stop(); stopErr != nil {
			if err == nil {
				err = fmt.Errorf("error stopping watcher: %v", stopErr)
			} else {
				logger.Warningf("ignoring error when stopping watcher: %v", stopErr)
			}
		}
		for len(p.machines) > 0 {
			delete(p.machines, (<-p.machineDead).Id())
		}
	}()
	for {
		select {
		case ids, ok := <-w.Changes():
			if !ok {
				return watcher.MustErr(w)
			}
			if err := p.startMachines(ids); err != nil {
				return err
			}
		case m := <-p.machineDead:
			delete(p.machines, m.Id())
		case <-p.context.dying():
			return nil
		}
	}
}

func (p *updater) startMachines(ids []string) error {
	for _, id := range ids {
		if c := p.machines[id]; c == nil {
			// We don't know about the machine - start
			// a goroutine to deal with it.
			m, err := p.context.getMachine(id)
			if errors.IsNotFoundError(err) {
				logger.Warningf("watcher gave notification of non-existent machine %q", id)
				continue
			}
			if err != nil {
				return err
			}
			c = make(chan struct{})
			p.machines[id] = c
			go runMachine(p.context.newMachineContext(), m, c, p.machineDead)
		} else {
			c <- struct{}{}
		}
	}
	return nil
}

// runMachine processes the address publishing for a given machine.
// We assume that the machine is alive when this is first called.
func runMachine(context machineContext, m machine, changed <-chan struct{}, died chan<- machine) {
	defer func() {
		// We can't just send on the died channel because the
		// central loop might be trying to write to us on the
		// changed channel.
		for {
			select {
			case died <- m:
				return
			case <-changed:
			}
		}
	}()
	if err := machineLoop(context, m, changed); err != nil {
		context.killAll(err)
	}
}

func machineLoop(context machineContext, m machine, changed <-chan struct{}) error {
	// Use a short poll interval when initially waiting for
	// a machine's address, and a long one when it already
	// has an address.
	pollInterval := longPoll
	if len(m.Addresses()) == 0 {
		pollInterval = shortPoll
	}
	for {
		instId, err := m.InstanceId()
		if err != nil {
			return fmt.Errorf("cannot get machine's instance id: %v", err)
		}
		newAddrs, err := context.addresses(instId)
		if err != nil {
			logger.Warningf("cannot get addresses for instance %q: %v", instId, err)
		} else if !addressesEqual(m.Addresses(), newAddrs) {
			if err := m.SetAddresses(newAddrs); err != nil {
				return fmt.Errorf("cannot set addresses on %q: %v", m, err)
			}
			pollInterval = longPoll
		}
		select {
		case <-time.After(pollInterval):
		case <-context.dying():
			return nil
		case <-changed:
			if err := m.Refresh(); err != nil {
				return err
			}
			// In practice the only event that will trigger
			// a change is the life state changing to dying or dead,
			// in which case we return. The logic will still work
			// if a change is triggered for some other reason,
			// but we don't mind an extra address check in that case,
			// seeing as it's unlikely.
			if m.Life() == state.Dead {
				return nil
			}
		}
	}
}

func addressesEqual(a0, a1 []instance.Address) bool {
	if len(a0) != len(a1) {
		return false
	}
	for i := range a0 {
		if a0[i] != a1[i] {
			return false
		}
	}
	return true
}
