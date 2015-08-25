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
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/instance"
	"github.com/juju/juju/network"
	"github.com/juju/juju/provider/dummy"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

// mockState implements StateInterface and allows inspection of called
// methods.
type mockState struct {
	mu   sync.Mutex
	stub *testing.Stub

	config      *config.Config
	ipAddresses map[string]*mockIPAddress

	ipAddressWatchers []*mockIPAddressWatcher
}

func newMockState() *mockState {
	mst := &mockState{
		stub:        &testing.Stub{},
		ipAddresses: make(map[string]*mockIPAddress),
	}
	mst.setUpState()
	return mst
}

func (mst *mockState) setUpState() {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	ips := []struct {
		value      string
		uuid       string
		life       state.Life
		subnetId   string
		instanceId string
		macaddr    string
	}{
		{"0.1.2.3", "00000000-1111-2222-3333-0123456789ab", state.Alive, "a", "a3", "fff3"},
		{"0.1.2.4", "00000000-1111-2222-4444-0123456789ab", state.Alive, "b", "b4", "fff4"},
		{"0.1.2.5", "00000000-1111-2222-5555-0123456789ab", state.Alive, "b", "b5", "fff5"},
		{"0.1.2.6", "00000000-1111-2222-6666-0123456789ab", state.Dead, "c", "c6", "fff6"},
		{"0.1.2.7", "00000000-1111-2222-7777-0123456789ab", state.Dead, "c", "c7", "fff7"},
	}
	for _, ip := range ips {
		mst.ipAddresses[ip.value] = &mockIPAddress{
			stub:       mst.stub,
			st:         mst,
			value:      ip.value,
			tag:        names.NewIPAddressTag(ip.uuid),
			life:       ip.life,
			subnetId:   ip.subnetId,
			instanceId: instance.Id(ip.instanceId),
			addr:       network.NewAddress(ip.value),
			macaddr:    ip.macaddr,
		}
	}
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

// setDead sets a mock IP address in state to dead.
func (mst *mockState) setDead(c *gc.C, value string) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	ipAddress, found := mst.ipAddresses[value]
	c.Assert(found, gc.Equals, true)

	ipAddress.life = state.Dead
}

// DeadIPAddresses implements StateInterface.
func (mst *mockState) DeadIPAddresses() ([]addresser.StateIPAddress, error) {
	mst.mu.Lock()
	defer mst.mu.Unlock()

	mst.stub.MethodCall(mst, "DeadIPAddresses")
	if err := mst.stub.NextErr(); err != nil {
		return nil, err
	}
	var deadIPAddresses []addresser.StateIPAddress
	for _, ipAddress := range mst.ipAddresses {
		if ipAddress.life == state.Dead {
			deadIPAddresses = append(deadIPAddresses, ipAddress)
		}
	}
	return deadIPAddresses, nil
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

// mockIPAddress implements StateIPAddress for testing.
type mockIPAddress struct {
	addresser.StateIPAddress

	stub       *testing.Stub
	st         *mockState
	value      string
	tag        names.Tag
	life       state.Life
	subnetId   string
	instanceId instance.Id
	addr       network.Address
	macaddr    string
}

var _ addresser.StateIPAddress = (*mockIPAddress)(nil)

// Value implements StateIPAddress.
func (mip *mockIPAddress) Value() string {
	mip.stub.MethodCall(mip, "Value")
	mip.stub.NextErr() // Consume the unused error.
	return mip.value
}

// Tag implements StateIPAddress.
func (mip *mockIPAddress) Tag() names.Tag {
	mip.stub.MethodCall(mip, "Tag")
	mip.stub.NextErr() // Consume the unused error.
	return mip.tag
}

// Life implements StateIPAddress.
func (mip *mockIPAddress) Life() state.Life {
	mip.stub.MethodCall(mip, "Life")
	mip.stub.NextErr() // Consume the unused error.
	return mip.life
}

// Remove implements StateIPAddress.
func (mip *mockIPAddress) Remove() error {
	mip.stub.MethodCall(mip, "Remove")
	if err := mip.stub.NextErr(); err != nil {
		return err
	}
	return mip.st.removeIPAddress(mip.value)
}

// SubnetId implements StateIPAddress.
func (mip *mockIPAddress) SubnetId() string {
	mip.stub.MethodCall(mip, "SubnetId")
	mip.stub.NextErr() // Consume the unused error.
	return mip.subnetId
}

// InstanceId implements StateIPAddress.
func (mip *mockIPAddress) InstanceId() instance.Id {
	mip.stub.MethodCall(mip, "InstanceId")
	mip.stub.NextErr() // Consume the unused error.
	return mip.instanceId
}

// Address implements StateIPAddress.
func (mip *mockIPAddress) Address() network.Address {
	mip.stub.MethodCall(mip, "Address")
	mip.stub.NextErr() // Consume the unused error.
	return mip.addr
}

// MACAddress implements StateIPAddress.
func (mip *mockIPAddress) MACAddress() string {
	mip.stub.MethodCall(mip, "MACAddress")
	mip.stub.NextErr() // Consume the unused error.
	return mip.macaddr
}

// mockIPAddressWatcher notifies about IP address changes.
type mockIPAddressWatcher struct {
	err       error
	initial   []string
	wontStart bool
	incoming  chan []string
	changes   chan []string
	done      chan struct{}
}

var _ state.StringsWatcher = (*mockIPAddressWatcher)(nil)

func newMockIPAddressWatcher(initial []string) *mockIPAddressWatcher {
	mipw := &mockIPAddressWatcher{
		initial:   initial,
		wontStart: false,
		incoming:  make(chan []string),
		changes:   make(chan []string),
		done:      make(chan struct{}),
	}
	return mipw
}

// Kill implements state.Watcher.
func (mipw *mockIPAddressWatcher) Kill() {}

// Stop implements state.Watcher.
func (mipw *mockIPAddressWatcher) Stop() error {
	select {
	case <-mipw.done:
		// Closed.
	default:
		// Signal the loop we want to stop.
		close(mipw.done)
		// Signal the clients we've closed.
		close(mipw.changes)
	}
	return mipw.err
}

// Wait implements state.Watcher.
func (mipw *mockIPAddressWatcher) Wait() error {
	return mipw.Stop()
}

// Err implements state.Watcher.
func (mipw *mockIPAddressWatcher) Err() error {
	return mipw.err
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

// mockConfig returns a configuration for the usage of the
// mock provider below.
func mockConfig() coretesting.Attrs {
	return dummy.SampleConfig().Merge(coretesting.Attrs{
		"type": "mock",
	})
}

// mockEnviron is an environment without networking support.
type mockEnviron struct {
	environs.Environ
}

func (e mockEnviron) Config() *config.Config {
	cfg, err := config.New(config.NoDefaults, mockConfig())
	if err != nil {
		panic("invalid configuration for testing")
	}
	return cfg
}

// mockEnvironProvider is the smallest possible provider to
// test the addresses without networking support.
type mockEnvironProvider struct {
	environs.EnvironProvider
}

func (p mockEnvironProvider) PrepareForBootstrap(ctx environs.BootstrapContext, cfg *config.Config) (environs.Environ, error) {
	return &mockEnviron{}, nil
}

func (p mockEnvironProvider) Open(*config.Config) (environs.Environ, error) {
	return &mockEnviron{}, nil
}
