// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"io"
	"sync"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/lease"
)

const (
	// CommandVersion is the current version of the command format. If
	// this changes then we need to be sure that reading and applying
	// commands for previous versions still works.
	CommandVersion = 1

	// SnapshotVersion is the current version of the snapshot
	// format. Similarly, changes to the snapshot representation need
	// to be backward-compatible.
	SnapshotVersion = 1

	// OperationClaim denotes claiming a new lease.
	OperationClaim = "claim"

	// OperationExtend denotes extending an already-held lease.
	OperationExtend = "extend"

	// OperationSetTime denotes updating stored global time (which
	// will also remove any expired leases).
	OperationSetTime = "setTime"

	// OperationPin pins a lease, preventing it from expiring
	// until it is unpinned.
	OperationPin = "pin"

	// OperationUnpin unpins a lease, restoring normal
	// lease expiry behaviour.
	OperationUnpin = "unpin"
)

// FSMResponse defines what will be available on the return value from
// FSM apply calls.
type FSMResponse interface {
	// Error is a lease error (rather than anything to do with the
	// raft machinery).
	Error() error

	// Notify tells the target what changes occurred because of the
	// applied command.
	Notify(NotifyTarget)
}

// NewFSM returns a new FSM to store lease information.
func NewFSM() *FSM {
	return &FSM{
		entries: make(map[lease.Key]*entry),
		pinned:  make(map[lease.Key]bool),
	}
}

// FSM stores the state of leases in the system.
type FSM struct {
	mu         sync.Mutex
	globalTime time.Time
	entries    map[lease.Key]*entry
	pinned     map[lease.Key]bool
}

func (f *FSM) claim(key lease.Key, holder string, duration time.Duration) *response {
	if _, found := f.entries[key]; found {
		return invalidResponse()
	}
	f.entries[key] = &entry{
		holder:   holder,
		start:    f.globalTime,
		duration: duration,
	}
	return &response{claimed: key, claimer: holder}
}

func (f *FSM) extend(key lease.Key, holder string, duration time.Duration) *response {
	entry, found := f.entries[key]
	if !found {
		return invalidResponse()
	}
	if entry.holder != holder {
		return invalidResponse()
	}
	expiry := f.globalTime.Add(duration)
	if !expiry.After(entry.start.Add(entry.duration)) {
		// No extension needed - the lease already expires after the
		// new time.
		return &response{}
	}
	// entry is a pointer back into the f.entries map, so this update
	// isn't lost.
	entry.start = f.globalTime
	entry.duration = duration
	return &response{}
}

func (f *FSM) pin(key lease.Key) *response {
	f.pinned[key] = true
	return &response{}
}

func (f *FSM) unpin(key lease.Key) *response {
	delete(f.pinned, key)
	return &response{}
}

func (f *FSM) setTime(oldTime, newTime time.Time) *response {
	if f.globalTime != oldTime {
		return &response{err: globalclock.ErrConcurrentUpdate}
	}
	f.globalTime = newTime
	return &response{expired: f.expired(newTime)}
}

// expired returns a collection of keys for leases that have expired.
// Any pinned leases are not included in the return.
func (f *FSM) expired(newTime time.Time) []lease.Key {
	var expired []lease.Key
	for key, entry := range f.entries {
		expiry := entry.start.Add(entry.duration)
		if expiry.Before(newTime) && !f.pinned[key] {
			delete(f.entries, key)
			expired = append(expired, key)
		}
	}
	return expired
}

// GlobalTime returns the FSM's internal time.
func (f *FSM) GlobalTime() time.Time {
	return f.globalTime
}

// Leases gets information about all of the leases in the system.
func (f *FSM) Leases(localTime time.Time) map[lease.Key]lease.Info {
	f.mu.Lock()
	results := make(map[lease.Key]lease.Info)
	for key, entry := range f.entries {
		globalExpiry := entry.start.Add(entry.duration)
		remaining := globalExpiry.Sub(f.globalTime)
		localExpiry := localTime.Add(remaining)
		results[key] = lease.Info{
			Holder: entry.holder,
			Expiry: localExpiry,
		}
	}
	f.mu.Unlock()
	return results
}

// entry holds the details of a lease.
type entry struct {
	// holder identifies the current holder of the lease.
	holder string

	// start is the global time at which the lease started.
	start time.Time

	// duration is the duration for which the lease is valid,
	// from the start time.
	duration time.Duration
}

var _ FSMResponse = (*response)(nil)

// response stores what happened as a result of applying a command.
type response struct {
	err     error
	claimer string
	claimed lease.Key
	expired []lease.Key
}

// Error is part of FSMResponse.
func (r *response) Error() error {
	return r.err
}

// Notify is part of FSMResponse.
func (r *response) Notify(target NotifyTarget) {
	if r.claimer != "" {
		target.Claimed(r.claimed, r.claimer)
	}
	for _, expiredKey := range r.expired {
		target.Expired(expiredKey)
	}
}

func invalidResponse() *response {
	return &response{err: lease.ErrInvalid}
}

// Apply is part of raft.FSM.
func (f *FSM) Apply(log *raft.Log) interface{} {
	var command Command
	err := yaml.Unmarshal(log.Data, &command)
	if err != nil {
		return &response{err: errors.Trace(err)}
	}
	if err := command.Validate(); err != nil {
		return &response{err: errors.Trace(err)}
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	switch command.Operation {
	case OperationClaim:
		return f.claim(command.LeaseKey(), command.Holder, command.Duration)
	case OperationExtend:
		return f.extend(command.LeaseKey(), command.Holder, command.Duration)
	case OperationPin:
		return f.pin(command.LeaseKey())
	case OperationUnpin:
		return f.unpin(command.LeaseKey())
	case OperationSetTime:
		return f.setTime(command.OldTime, command.NewTime)
	default:
		return &response{err: errors.NotValidf("operation %q", command.Operation)}
	}
}

// Snapshot is part of raft.FSM.
func (f *FSM) Snapshot() (raft.FSMSnapshot, error) {
	f.mu.Lock()

	entries := make(map[SnapshotKey]SnapshotEntry, len(f.entries))
	for key, entry := range f.entries {
		entries[SnapshotKey{
			Namespace: key.Namespace,
			ModelUUID: key.ModelUUID,
			Lease:     key.Lease,
		}] = SnapshotEntry{
			Holder:   entry.holder,
			Start:    entry.start,
			Duration: entry.duration,
		}
	}

	pinned := make(map[SnapshotKey]bool, len(f.pinned))
	for key := range f.pinned {
		pinned[SnapshotKey{
			Namespace: key.Namespace,
			ModelUUID: key.ModelUUID,
			Lease:     key.Lease,
		}] = true
	}

	f.mu.Unlock()

	return &Snapshot{
		Version:    SnapshotVersion,
		Entries:    entries,
		Pinned:     pinned,
		GlobalTime: f.globalTime,
	}, nil
}

// Restore is part of raft.FSM.
func (f *FSM) Restore(reader io.ReadCloser) error {
	defer reader.Close()

	var snapshot Snapshot
	decoder := yaml.NewDecoder(reader)
	if err := decoder.Decode(&snapshot); err != nil {
		return errors.Trace(err)
	}
	if snapshot.Version != SnapshotVersion {
		return errors.NotValidf("snapshot version %d", snapshot.Version)
	}
	if snapshot.Entries == nil {
		return errors.NotValidf("nil entries")
	}

	newEntries := make(map[lease.Key]*entry, len(snapshot.Entries))
	for key, ssEntry := range snapshot.Entries {
		newEntries[lease.Key{
			Namespace: key.Namespace,
			ModelUUID: key.ModelUUID,
			Lease:     key.Lease,
		}] = &entry{
			holder:   ssEntry.Holder,
			start:    ssEntry.Start,
			duration: ssEntry.Duration,
		}
	}

	newPinned := make(map[lease.Key]bool, len(snapshot.Pinned))
	for key := range snapshot.Pinned {
		newPinned[lease.Key{
			Namespace: key.Namespace,
			ModelUUID: key.ModelUUID,
			Lease:     key.Lease,
		}] = true
	}

	f.mu.Lock()
	f.globalTime = snapshot.GlobalTime
	f.entries = newEntries
	f.pinned = newPinned
	f.mu.Unlock()

	return nil
}

// Snapshot defines the format of the FSM snapshot.
type Snapshot struct {
	Version    int                           `yaml:"version"`
	Entries    map[SnapshotKey]SnapshotEntry `yaml:"entries"`
	Pinned     map[SnapshotKey]bool          `yaml:"pinned"`
	GlobalTime time.Time                     `yaml:"global-time"`
}

// Persist is part of raft.FSMSnapshot.
func (s *Snapshot) Persist(sink raft.SnapshotSink) (err error) {
	defer func() {
		if err != nil {
			sink.Cancel()
		}
	}()

	encoder := yaml.NewEncoder(sink)
	if err := encoder.Encode(s); err != nil {
		return errors.Trace(err)
	}
	if err := encoder.Close(); err != nil {
		return errors.Trace(err)
	}
	return sink.Close()
}

// Release is part of raft.FSMSnapshot.
func (s *Snapshot) Release() {}

// SnapshotKey defines the format of a lease key in a snapshot.
type SnapshotKey struct {
	Namespace string `yaml:"namespace"`
	ModelUUID string `yaml:"model-uuid"`
	Lease     string `yaml:"lease"`
}

// SnapshotEntry defines the format of a lease entry in a snapshot.
type SnapshotEntry struct {
	Holder   string        `yaml:"holder"`
	Start    time.Time     `yaml:"start"`
	Duration time.Duration `yaml:"duration"`
}

// Command captures the details of an operation to be run on the FSM.
type Command struct {
	// Version of the command format, in case it changes and we need
	// to handle multiple formats.
	Version int `yaml:"version"`

	// Operation is one of claim, extend, expire or setTime.
	Operation string `yaml:"operation"`

	// Namespace is the kind of lease.
	Namespace string `yaml:"namespace,omitempty"`

	// ModelUUID identifies the model the lease belongs to.
	ModelUUID string `yaml:"model-uuid,omitempty"`

	// Lease is the name of the lease the command affects.
	Lease string `yaml:"lease,omitempty"`

	// Holder is the name of the party claiming or extending the
	// lease.
	Holder string `yaml:"holder,omitempty"`

	// Duration is how long the lease should last.
	Duration time.Duration `yaml:"duration,omitempty"`

	// OldTime is the previous time for time updates (to avoid
	// applying stale ones).
	OldTime time.Time `yaml:"old-time,omitempty"`

	// NewTime is the time to store as the global time.
	NewTime time.Time `yaml:"new-time,omitempty"`
}

// Validate checks that the command describes a valid state change.
func (c *Command) Validate() error {
	// For now there's only version 1.
	if c.Version != 1 {
		return errors.NotValidf("version %d", c.Version)
	}
	switch c.Operation {
	case OperationClaim, OperationExtend:
		if err := c.validateLeaseKey(); err != nil {
			return err
		}
		if err := c.validateNoTime(); err != nil {
			return err
		}
		if c.Holder == "" {
			return errors.NotValidf("%s with empty holder", c.Operation)
		}
		if c.Duration == 0 {
			return errors.NotValidf("%s with zero duration", c.Operation)
		}
	case OperationPin, OperationUnpin:
		if err := c.validateLeaseKey(); err != nil {
			return err
		}
		if err := c.validateNoTime(); err != nil {
			return err
		}
		if c.Duration != 0 {
			return errors.NotValidf("pin with duration")
		}
	case OperationSetTime:
		// An old time of 0 is valid when starting up.
		var zeroTime time.Time
		if c.NewTime == zeroTime {
			return errors.NotValidf("setTime with zero new time")
		}
		if c.Holder != "" {
			return errors.NotValidf("setTime with holder")
		}
		if c.Duration != 0 {
			return errors.NotValidf("setTime with duration")
		}
		if c.Namespace != "" {
			return errors.NotValidf("setTime with namespace")
		}
		if c.ModelUUID != "" {
			return errors.NotValidf("setTime with model UUID")
		}
		if c.Lease != "" {
			if c.Holder == "" {
				return errors.NotValidf("%s with empty holder", c.Operation)
			}
			return errors.NotValidf("setTime with lease")
		}
	default:
		return errors.NotValidf("operation %q", c.Operation)
	}
	return nil
}

func (c *Command) validateLeaseKey() error {
	if c.Namespace == "" {
		return errors.NotValidf("%s with empty namespace", c.Operation)
	}
	if c.ModelUUID == "" {
		return errors.NotValidf("%s with empty model UUID", c.Operation)
	}
	if c.Lease == "" {
		return errors.NotValidf("%s with empty lease", c.Operation)
	}
	return nil
}

func (c *Command) validateNoTime() error {
	var zeroTime time.Time
	if c.OldTime != zeroTime {
		return errors.NotValidf("%s with old time", c.Operation)
	}
	if c.NewTime != zeroTime {
		return errors.NotValidf("%s with new time", c.Operation)
	}
	return nil
}

// LeaseKey makes a lease key from the fields in the command.
func (c *Command) LeaseKey() lease.Key {
	return lease.Key{
		Namespace: c.Namespace,
		ModelUUID: c.ModelUUID,
		Lease:     c.Lease,
	}
}

// Marshal converts this command to a byte slice.
func (c *Command) Marshal() ([]byte, error) {
	return yaml.Marshal(c)
}

// UnmarshalCommand converts a marshalled command []byte into a
// command.
func UnmarshalCommand(data []byte) (*Command, error) {
	var result Command
	err := yaml.Unmarshal(data, &result)
	return &result, err
}
