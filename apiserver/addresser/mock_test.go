// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"sort"
	"sync"

	"github.com/juju/errors"
	"github.com/juju/testing"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/addresser"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// ipAddressInfo contains the mocked information about an IP address.
type ipAddressInfo struct {
	id          string
	uuid        string
	value       string
	subnetId    string
	instanceId  string
	addressType network.AddressType
	scope       network.Scope
	life        state.Life
}

// mock implements StateIPAddress.
type mockIPAddress struct {
	*testing.Stub
	addresser.StateIPAddress

	st *mockState

	ipAddressInfo
}

var _ addresser.StateIPAddress = (*mockIPAddress)(nil)

// Value implements StateIPAddress.
func (a *mockIPAddress) Value() string {
	return a.value
}

// UUID implements StateIPAddress.
func (i *IPAddress) UUID() (utils.UUID, error) {
	return utils.UUIDFromString(a.uuid)
}

// Tag implements StateIPAddress.
func (i *IPAddress) Tag() names.Tag {
	return names.NewIPAddressTag(a.uuid)
}

// SubnetId implements StateIPAddress.
func (a *mockIPAddress) SubnetId() string {
	return a.subnetId
}

// InstanceId implements StateIPAddress.
func (a *mockIPAddress) InstanceId() instance.Id {
	return instance.Id(a.instanceId)
}

// Type implements StateIPAddress.
func (a *mockIPAddress) Type() network.AddressType {
	return a.addressType
}

// Scope implements StateIPAddress.
func (a *mockIPAddress) Scope() network.Scope {
	return a.scope
}

// Life implements StateIPAddress.
func (a *mockIPAddress) Life() state.Life {
	return a.life
}

// Remove implements StateIPAddress.
func (a *mockIPAddress) Remove() error {
	return a.st.run(func() (err error) {
		defer errors.DeferredAnnotatef(&err, "cannot remove IP address %q", a.id)
		ra, ok := a.st.ipAddresses[a.id]
		if !ok {
			// Different from original one!
			return errors.NotFoundf("IP address %q", a.id)
		}
		if ra.life != state.Dead {
			return errors.New("IP address is not dead")
		}
		delete(a.st.ipAddresses, a.id)
		return nil
	})
}

// mockIPAddressWatcher
type mockIPAddressWatcher struct {
	err       error
	initial   []string
	incomingc chan []string
	changesc  chan []string
	donec     chan struct{}
}

func newMockIPAddressWatcher(initial []string, err error) *mockIPAddressWatcher {
	w := &mockIPAddressWatcher{
		err:       err,
		initial:   initial,
		incomingc: make(chan []string),
		changesc:  make(chan []string),
		donec:     make(chan struct{}),
	}
	if err == nil {
		go w.loop()
	}
	return w
}

// Kill implements state.Watcher.
func (w *mockIPAddressWatcher) Kill() {}

// Stop implements state.Watcher.
func (w *mockIPAddressWatcher) Stop() error {
	select {
	case <-w.donec:
		// Already closed.
	default:
		close(w.donec)
		close(w.changesc)
	}
	return w.err
}

// Wait implements state.Watcher.
func (w *mockIPAddressWatcher) Wait() error {
	return w.Stop()
}

// Err implements state.Watcher.
func (w *mockIPAddressWatcher) Err() error {
	return w.err
}

// Changes implements state.StringsWatcher.
func (w *mockIPAddressWatcher) Changes() <-chan []string {
	return w.changesc
}

func (w *mockIPAddressWatcher) loop() {
	unsent := w.initial
	outChangesC := w.changesc
	for {
		select {
		case <-w.donec:
			return
		case outChangesC <- unsent:
			outChangesC = nil
			unsent = nil
		case ids := <-w.incomingc:
			unsent = append(unsent, ids...)
			outChangesC = w.changesc
		}
	}
}

// mockState implements StateInterface and allows inspection of called
// methods.
type mockState struct {
	*testing.Stub

	mu sync.Mutex

	config            *config.Config
	ipAddresses       map[names.Tag]*mockIPAddress
	ipAddressWatchers []*mockIPAddressWatcher
}

func NewMockState(c *gc.C) *mockState {
	st := &mockState{
		Stub:        &testing.Stub{},
		ipAddresses: make(map[string]*mockIPAddress),
	}
	st.setEnvironConfig(coretesting.EnvironConfig(c))
	st.addIPAddresses()
	return st
}

var _ addresser.StateInterface = (*mockState)(nil)

// EnvironConfig implements StateInterface.
func (st *mockState) EnvironConfig() (*config.Config, error) {
	err := st.run(func() error {
		st.MethodCall(st, "EnvironConfig")
		return st.NextErr()
	})
	if err != nil {
		return nil, err
	}
	return st.config, nil
}

// FindEntity implements the StateInterface.
func (st *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	var e state.Entity
	err := st.run(func() error {
		if tag.Kind() == IPAddressTagKind {
			found, ok := st.ipAddresses[tag]
			if !ok {
				return errors.NotFoundf("IP address %q", id)
			}
			e = found
			return nil
		}
	})
	if err != nil {
		return nil, err
	}
	return e, nil
}

// WatchIPAddresses implements the StateInterface.
func (st *mockState) WatchIPAddresses() state.StringsWatcher {
	var w *mockIPAddressWatcher
	st.run(func() error {
		st.MethodCall(st, "WatchIPAddresses")
		ids := make([]string, 0, len(st.ipAddresses))
		// Initial event with all IP addresses sorted.
		for id := range st.ipAddresses {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		w = newMockIPAddressWatcher(ids, st.NextErr())
		st.ipAddressWatchers = append(st.ipAddressWatchers, w)
		return nil
	})
	return w
}

func (st *mockState) setEnvironConfig(config *config.Config) {
	st.run(func() error {
		st.config = config
		return nil
	})
}

func (st *mockState) addIPAddresses() {
	addrs := []struct {
		ip   string
		uuid string
		snid string
		iid  string
		life state.Life
	}{
		{"0.1.2.3", "1-2-3-4-a", "foo-3", "bar-3", state.Alive},
		{"0.1.2.4", "1-2-3-4-b", "foo-4", "bar-4", state.Alive},
		{"0.1.2.5", "1-2-3-4-c", "foo-5", "bar-5", state.Alive},
		{"0.1.2.6", "1-2-3-4-d", "foo-6", "bar-6", state.Dead},
		{"0.1.2.7", "1-2-3-4-e", "foo-7", "bar-7", state.Dead},
	}
	for _, addr := range addrs {
		st.run(func() error {
			ipAddress = &mockIPAddress{
				st: st,
				ipAddressInfo: ipAddressInfo{
					id:          addr.ip,
					uuid:        addr.uuid,
					value:       addr.ip,
					subnetId:    addr.snid,
					instanceId:  addr.iid,
					addressType: network.IPv4Address,
					scope:       network.ScopePublic,
					life:        addr.life,
				},
			}
			st.ipAddresses[ipAddress.Tag()] = ipAddress
			return nil
		})
	}
}

// run simulates a transactional behavior using the mock
// states mutex.
func (st *mockState) run(f func() error) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return f()
}
