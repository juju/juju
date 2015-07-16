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
	mu   sync.Mutex
	stub *testing.Stub

	config      *config.Config
	ipAddresses map[string]*mockIPAddress

	configWatchers    []*mockConfigWatcher
	ipAddressWatchers []*mockIPAddressWatcher
}

func newMockState() *mockState {
	mst := &mockState{
		stub:        &testing.Stub{},
		ipAddresses: make(map[string]*mockIPAddress),
	}
	mst.setUpIPAddresses()
	return mst
}

var _ addresser.StateInterface = (*mockState)(nil)

// EnvironConfig implements StateInterface.
func (mst *mockState) EnvironConfig() (*config.Config, error) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.stub.MethodCall(mst, "EnvironConfig")

	if err := mst.stub.NextErr(); err != nil {
		return nil, err
	}
	return mst.config, nil
}

// setConfig updates the environ config stored internally. Triggers a
// change event for all created config watchers.
func (mst *mockState) setConfig(c *gc.C, newConfig *config.Config) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.config = newConfig

	// Notify any watchers for the changes.
	for _, w := range mst.configWatchers {
		w.incoming <- struct{}{}
	}
}

// WatchForEnvironConfigChanges implements StateInterface.
func (mst *mockState) WatchForEnvironConfigChanges() state.NotifyWatcher {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.stub.MethodCall(mst, "WatchForEnvironConfigChanges")
	mst.stub.NextErr()
	return mst.nextEnvironConfigChangesWatcher()
}

// addEnvironConfigChangesWatcher adds an environment config changes watches.
func (mst *mockState) addEnvironConfigChangesWatcher(err error) *mockConfigWatcher {
	w := newMockConfigWatcher(err)
	mst.configWatchers = append(mst.configWatchers, w)
	return w
}

// nextEnvironConfigChangesWatcher returns an environment
// config changes watcher.
func (mst *mockState) nextEnvironConfigChangesWatcher() *mockConfigWatcher {
	if len(mst.configWatchers) == 0 {
		panic("ran out of watchers")
	}

	w := mst.configWatchers[0]
	mst.configWatchers = mst.configWatchers[1:]
	w.start()
	return w
}

// FindEntity implements StateInterface.
func (mst *mockState) FindEntity(tag names.Tag) (state.Entity, error) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.stub.MethodCall(mst, "FindEntity", tag)

	if err := mst.stub.NextErr(); err != nil {
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

	mst.stub.MethodCall(mst, "IPAddress", value)

	if err := mst.stub.NextErr(); err != nil {
		return nil, err
	}
	ipAddress, found := mst.ipAddresses[value]
	if !found {
		return nil, errors.NotFoundf("IP address %s", value)
	}
	return ipAddress, nil
}

// WatchIPAddresses implements StateInterface.
func (mst *mockState) WatchIPAddresses() state.StringsWatcher {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.stub.MethodCall(mst, "WatchIPAddresses")
	mst.stub.NextErr()
	return mst.nextIPAddressWatcher()
}

// addIPAddressWatcher adds an IP address watcher with the given IDs.
func (mst *mockState) addIPAddressWatcher(ids ...string) *mockIPAddressWatcher {
	w := newMockIPAddressWatcher(ids)
	mst.ipAddressWatchers = append(mst.ipAddressWatchers, w)
	return w
}

// nextIPAddressWatcher returns an IP address watcher .
func (mst *mockState) nextIPAddressWatcher() *mockIPAddressWatcher {
	if len(mst.ipAddressWatchers) == 0 {
		panic("ran out of watchers")
	}

	w := mst.ipAddressWatchers[0]
	mst.ipAddressWatchers = mst.ipAddressWatchers[1:]
	if len(w.initial) == 0 {
		ids := make([]string, 0, len(mst.ipAddresses))
		// Initial event - all IP address ids, sorted.
		for id := range mst.ipAddresses {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		w.initial = ids
	}
	w.start()
	return w
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
			stub:  mst.stub,
			st:    mst,
			value: ip.value,
			tag:   names.NewIPAddressTag(ip.uuid),
			life:  ip.life,
		}
	}
}

// mockIPAddress implements StateIPAddress for testing.
type mockIPAddress struct {
	addresser.StateIPAddress

	mu   sync.Mutex
	stub *testing.Stub

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

	mip.stub.MethodCall(mip, "Tag")
	mip.stub.NextErr() // Consume the unused error.
	return mip.tag
}

// Life implements StateIPAddress.
func (mip *mockIPAddress) Life() state.Life {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.stub.MethodCall(mip, "Life")
	mip.stub.NextErr() // Consume the unused error.
	return mip.life
}

// EnsureDead implements StateIPAddress.
func (mip *mockIPAddress) EnsureDead() error {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.stub.MethodCall(mip, "EnsureDead")
	if err := mip.stub.NextErr(); err != nil {
		return err
	}
	mip.life = state.Dead
	return nil
}

// Remove implements StateIPAddress.
func (mip *mockIPAddress) Remove() error {
	mip.mu.Lock()
	defer mip.mu.Unlock()

	mip.stub.MethodCall(mip, "Remove")
	if err := mip.stub.NextErr(); err != nil {
		return err
	}
	return mip.st.removeIPAddress(mip.value)
}

// mockBaseWatcher is a helper for the watcher mocks.
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

	wontStart bool
	incoming  chan struct{}
	changes   chan struct{}
}

var _ state.NotifyWatcher = (*mockConfigWatcher)(nil)

func newMockConfigWatcher(err error) *mockConfigWatcher {
	changes := make(chan struct{})
	mcw := &mockConfigWatcher{
		wontStart:       false,
		incoming:        make(chan struct{}),
		changes:         changes,
		mockBaseWatcher: newMockBaseWatcher(err, func() { close(changes) }),
	}
	return mcw
}

// start starts the backend loop depending on a field setting.
func (mcw *mockConfigWatcher) start() {
	if mcw.wontStart {
		// Set manually by tests that need it.
		mcw.Stop()
		return
	}
	go mcw.loop()
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

	initial   []string
	wontStart bool
	incoming  chan []string
	changes   chan []string
}

var _ state.StringsWatcher = (*mockIPAddressWatcher)(nil)

func newMockIPAddressWatcher(initial []string) *mockIPAddressWatcher {
	changes := make(chan []string)
	mipw := &mockIPAddressWatcher{
		initial:         initial,
		wontStart:       false,
		changes:         changes,
		incoming:        make(chan []string),
		mockBaseWatcher: newMockBaseWatcher(nil, func() { close(changes) }),
	}
	return mipw
}

// start starts the backend loop depending on a field setting.
func (mipw *mockIPAddressWatcher) start() {
	if mipw.wontStart {
		// Set manually by tests that need it.
		mipw.Stop()
		return
	}
	go mipw.loop()
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
