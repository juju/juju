package addresspublisher

import (
	"fmt"
	"sync"
	"time"

	"launchpad.net/loggo"
	"launchpad.net/tomb"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/instance"
	"launchpad.net/juju-core/state"
)

var logger = loggo.GetLogger("juju.worker.addresspublished")

//func NewAddressPublisher() worker.Worker {
//	p := &publisher{
//		st:
//	}
//	// wait for environment
//	go func() {
//		defer p.tomb.Done()
//		p.tomb.Kill(p.loop())
//	}()
//}

type publisher struct {
	st   *state.State
	tomb tomb.Tomb

	mu      sync.Mutex
	environ environs.Environ
}

type machine interface {
	Addresses() []instance.Address
	InstanceId() (instance.Id, error)
	SetAddresses([]instance.Address) error
	Jobs() []state.MachineJob
	String() string
	Refresh() error
	Life() state.Life
}

var _ machine = (*state.Machine)(nil)

//func (p *publisher) loop() error {
//	w := p.st.WatchEnvironMachines()
//	machines := make(map[string] chan struct{})
//	machineDead := make(chan string)
////	publishFunc := func(addrs []
////	publisherc := make(chan machineAddress)
////	go publisher(publisherc)
//	defer func() {
//		w.Stop()
//		for len(machines) > 0 {
//			delete(machines, <-machineDead)
//		}
////		close(publisherc)
//	}()
//	for {
//		select {
//		case ids, ok  := <-w.Changes():
//			if !ok {
//				return watcher.MustErr(w)
//			}
//			for _, id := range ids {
//				c := machines[id]
//				if c == nil {
//					// We don't know about the machine - start
//					// a goroutine to deal with it.
//					c = make(chan struct{})
//					machines[id] = c
//					go machine(id, c, machineDead)
//				}
//				c <- struct{}{}
//			}
//		case id := <-machineDead:
//			delete(machines, id)
//		case <-tomb.Dying():
//			return nil
//		}
//	}
//}

const (
	longPoll  = 10 * time.Second
	shortPoll = 500 * time.Millisecond
)

type machineContext interface {
	killAll(err error)
	instance(id instance.Id) (instance.Instance, error)
	dying() <-chan struct{}
}

func runMachine(ctxt machineContext, m machine, changed <-chan struct{}, died chan<- machine, publisherc chan<- machineAddress) {
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
	if err := machineLoop(ctxt, m, changed, publisherc); err != nil {
		ctxt.killAll(err)
	}
}

func machineLoop(ctxt machineContext, m machine, changed <-chan struct{}, publisherc chan<- machineAddress) error {
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
		inst, err := ctxt.instance(instId)
		if err != nil {
			logger.Warningf("cannot get instance for %q: %v", instId, err)
			continue
		}
		newAddrs, err := inst.Addresses()
		if err != nil {
			logger.Warningf("cannot get addresses for instance: %v", err)
			continue
		}

		if !addressesEqual(m.Addresses(), newAddrs) {
			if err := m.SetAddresses(newAddrs); err != nil {
				return fmt.Errorf("cannot set addresses on %q: %v", m, err)
			}
			pollInterval = longPoll
			publisherc <- machineAddress{
				machine:   m,
				addresses: newAddrs,
			}
		}
		select {
		case <-time.After(pollInterval):
		case <-ctxt.dying():
			return nil
		case <-changed:
			if err := m.Refresh(); err != nil {
				return err
			}
			if m.Life() == state.Dying || m.Life() == state.Dead {
				publisherc <- machineAddress{machine: m}
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

type machineAddress struct {
	machine   machine
	addresses []instance.Address
}

//
//func publisher(c <-chan machineAddress, publish func(addresses []string)) {
//	addresses := set.NewStrings()
//	stateServers := make(map[machine] machineAddress)
//	for addr := range c {
//		if !hasJob(m, state.JobServeState) {
//			continue
//		}
//		old := stateServers[addr.machine]
//		if addr.address == "" {
//			addresses.Delete(old.address)
//			delete(stateServers, addr.machine)
//		} else {
//			addresses.Add(addr.address)
//			stateServers[addr.machine] = addr
//		}
//		publish(addresses)
//	}
//}
