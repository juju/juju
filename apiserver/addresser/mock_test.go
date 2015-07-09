// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package addresser_test

import (
	"sort"
	"sync"

	gc "gopkg.in/check.v1"

	"github.com/juju/errors"
	"github.com/juju/names"
	"github.com/juju/testing"
	jujutxn "github.com/juju/txn"

	"github.com/juju/juju/apiserver/addresser"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
)

// mockState implements StateInterface and allows inspection of called
// methods.
type mockState struct {
	*testing.Stub

	mu sync.Mutex

	configWatchers    []*mockConfigWatcher
	ipAddressWatchers []*mockIPAddressWatcher

	config      *config.Config
	ipAddresses map[string]*mockIPAddress
}

func NewMockState() *mockState {
	mst := &mockState{
		Stub:        &testing.Stub{},
		ipAddresses: make(map[string]*mockIPAddress),
	}
	mst.setUpIPAddresses()
	return mst
}

var _ addresser.StateInterface = (*mockState)(nil)

// CheckFindEntityCall is a helper wrapper aroud
// testing.Stub.CheckCall for FindEntity.
func (mst *mockState) CheckFindEntityCall(c *gc.C, index int, id string) {
	mst.CheckCall(c, index, "FindEntity", interface{}(names.NewIPAddressTag(id)))
}

// WatchForEnvironConfigChanges implements StateInterface.
func (mst *mockState) WatchForEnvironConfigChanges() state.NotifyWatcher {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.MethodCall(mst, "WatchForEnvironConfigChanges")

	w := newMockConfigWatcher(mst.NextErr())
	mst.configWatchers = append(mst.configWatchers, w)
	return w
}

// EnvironConfig implements StateInterface.
func (mst *mockState) EnvironConfig() (*config.Config, error) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.MethodCall(mst, "EnvironConfig")

	if err := mst.NextErr(); err != nil {
		return nil, err
	}
	return mst.config, nil
}

// SetConfig updates the environ config stored internally. Triggers a
// change event for all created config watchers.
func (mst *mockState) SetConfig(c *gc.C, newConfig *config.Config) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.config = newConfig

	// Notify any watchers for the changes.
	for _, w := range mst.configWatchers {
		w.incoming <- struct{}{}
	}
}

// WatchIPAddresses implements StateInterface.
func (mst *mockState) WatchIPAddresses() state.StringsWatcher {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.MethodCall(mst, "WatchIPAddresses")

	ids := make([]string, 0, len(mst.ipAddresses))
	// Initial event - all IP address ids, sorted.
	for id := range mst.ipAddresses {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	w := newMockIPAddressWatcher(ids, mst.NextErr())
	mst.ipAddressWatchers = append(mst.ipAddressWatchers, w)
	return w
}

// FindEntity implements StateInterface.
func (mst *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.MethodCall(mst, "FindEntity", tag)

	if err := mst.NextErr(); err != nil {
		return nil, err
	}
	if tag == nil {
		return nil, errors.NotValidf("nil tag is") // +" not valid"
	}
	for _, ipAddress := range mst.ipAddresses {
		if ipAddress.Tag().String() == tag.String() {
			return ipAddress, nil
		}
	}
	return nil, errors.NotFoundf("IP address %s", tag)
}

// IPAddress implements StateInterface.
func (mst *mockState) IPAddress(value string) (addresser.StateIPAddress, error) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.MethodCall(mst, "IPAddress", value)

	if err := mst.NextErr(); err != nil {
		return nil, err
	}
	ipAddress, found := mst.ipAddresses[value]
	if !found {
		return nil, errors.NotFoundf("IP address %s", value)
	}
	return ipAddress, nil
}

// removeIPAddress is used by mockIPAddress.Remove()
func (mst *mockState) removeIPAddress(value string) error {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	ipAddr, ok := mst.ipAddresses[value]
	if !ok {
		return jujutxn.ErrNoOperations
	}
	if ipAddr.life != state.Dead {
		return errors.Errorf("cannot remove IP address %q: IP address is not dead", ipAddr.value)
	}
	delete(mst.ipAddresses, value)
	return nil
}

// StartSync implements statetesting.SyncStarter, so mockState can be
// used with watcher helpers/checkers.
func (mst *mockState) StartSync() {}

func (mst *mockState) setUpIPAddresses() {
	ips := []struct {
		value string
		uuid  string
		life  state.Life
	}{
		{"0.1.2.3", "00000000-1111-2222-3333-0123456789ab", state.Alive},
		{"0.1.2.4", "00000000-1111-2222-4444-0123456789ab", state.Alive},
		{"0.1.2.5", "00000000-1111-2222-5555-0123456789ab", state.Alive},
		{"0.1.2.6", "00000000-1111-2222-6666-0123456789ab", state.Dead},
		{"0.1.2.7", "00000000-1111-2222-7777-0123456789ab", state.Dead},
	}
	for _, ip := range ips {
		mst.ipAddresses[ip.value] = &mockIPAddress{
			Stub:  mst.Stub,
			st:    mst,
			value: ip.value,
			tag:   names.NewIPAddressTag(ip.uuid),
			life:  ip.life,
		}
	}
}

// mockIPAddress implements StateIPAddress for testing.
type mockIPAddress struct {
	*testing.Stub
	addresser.StateIPAddress

	mu sync.Mutex

	st    *mockState
	value string
	tag   names.Tag
	life  state.Life
}

var _ addresser.StateIPAddress = (*mockIPAddress)(nil)

// Tag implements StateIPAddress.
func (mip *mockIPAddress) Tag() names.Tag {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.MethodCall(mip, "Tag")
	mip.NextErr() // Consume the unused error.
	return mip.tag
}

// Life implements StateIPAddress.
func (mip *mockIPAddress) Life() state.Life {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.MethodCall(mip, "Life")
	mip.NextErr() // Consume the unused error.
	return mip.life
}

// EnsureDead implements StateIPAddress.
func (mip *mockIPAddress) EnsureDead() error {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.MethodCall(mip, "EnsureDead")
	mip.life = state.Dead
	return mip.NextErr()
}

// Remove implements StateIPAddress.
func (mip *mockIPAddress) Remove() error {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.MethodCall(mip, "Remove")

	return mip.st.removeIPAddress(mip.value)
}

type mockBaseWatcher struct {
	err error

	closeChanges func()
	done         chan struct{}
}

var _ state.Watcher = (*mockBaseWatcher)(nil)

func newMockBaseWatcher(err error, closeChanges func()) *mockBaseWatcher {
	w := &mockBaseWatcher{
		err:          err,
		closeChanges: closeChanges,
		done:         make(chan struct{}),
	}
	if err != nil {
		w.Stop()
	}
	return w
}

// Kill implements state.Watcher.
func (mbw *mockBaseWatcher) Kill() {}

// Stop implements state.Watcher.
func (mbw *mockBaseWatcher) Stop() error {
	select {
	case <-mbw.done:
		// Closed.
	default:
		// Signal the loop we want to stop.
		close(mbw.done)
		// Signal the clients we've closed.
		mbw.closeChanges()
	}
	return mbw.err
}

// Wait implements state.Watcher.
func (mbw *mockBaseWatcher) Wait() error {
	return mbw.Stop()
}

// Err implements state.Watcher.
func (mbw *mockBaseWatcher) Err() error {
	return mbw.err
}

// mockConfigWatcher notifies about config changes.
type mockConfigWatcher struct {
	*mockBaseWatcher

	incoming chan struct{}
	changes  chan struct{}
}

var _ state.NotifyWatcher = (*mockConfigWatcher)(nil)

func newMockConfigWatcher(err error) *mockConfigWatcher {
	changes := make(chan struct{})
	mcw := &mockConfigWatcher{
		changes:         changes,
		incoming:        make(chan struct{}),
		mockBaseWatcher: newMockBaseWatcher(err, func() { close(changes) }),
	}
	if err == nil {
		go mcw.loop()
	}
	return mcw
}

func (mcw *mockConfigWatcher) loop() {
	// Prepare initial event.
	outChanges := mcw.changes
	// Forward any incoming changes until stopped.
	for {
		select {
		case <-mcw.done:
			return
		case outChanges <- struct{}{}:
			outChanges = nil
		case <-mcw.incoming:
			outChanges = mcw.changes
		}
	}
}

// Changes implements state.NotifyWatcher.
func (mcw *mockConfigWatcher) Changes() <-chan struct{} {
	return mcw.changes
}

// mockIPAddressWatcher notifies about IP address changes.
type mockIPAddressWatcher struct {
	*mockBaseWatcher

	initial  []string
	incoming chan []string
	changes  chan []string
}

var _ state.StringsWatcher = (*mockIPAddressWatcher)(nil)

func newMockIPAddressWatcher(initial []string, err error) *mockIPAddressWatcher {
	changes := make(chan []string)
	mipw := &mockIPAddressWatcher{
		initial:         initial,
		changes:         changes,
		incoming:        make(chan []string),
		mockBaseWatcher: newMockBaseWatcher(err, func() { close(changes) }),
	}
	if err == nil {
		go mipw.loop()
	}
	return mipw
}

func (mipw *mockIPAddressWatcher) loop() {
	// Prepare initial event.
	unsent := mipw.initial
	outChanges := mipw.changes
	// Forward any incoming changes until stopped.
	for {
		select {
		case <-mipw.done:
			return
		case outChanges <- unsent:
			outChanges = nil
			unsent = nil
		case ids := <-mipw.incoming:
			unsent = append(unsent, ids...)
			outChanges = mipw.changes
		}
	}
}

// Changes implements state.StringsWatcher.
func (mipw *mockIPAddressWatcher) Changes() <-chan []string {
	return mipw.changes
}
