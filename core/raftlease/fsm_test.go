// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease_test

import (
	"bytes"
	"io"
	"time"

	"github.com/hashicorp/raft"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/lease"
	"github.com/juju/juju/core/raftlease"
)

var zero time.Time

type fsmSuite struct {
	testing.IsolationSuite

	fsm *raftlease.FSM
}

var _ = gc.Suite(&fsmSuite{})

func (s *fsmSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.fsm = raftlease.NewFSM()
}

func (s *fsmSuite) apply(c *gc.C, command raftlease.Command) raftlease.FSMResponse {
	data, err := command.Marshal()
	c.Assert(err, jc.ErrorIsNil)
	result := s.fsm.Apply(&raft.Log{Data: data})
	response, ok := result.(raftlease.FSMResponse)
	c.Assert(ok, gc.Equals, true)
	return response
}

func (s *fsmSuite) TestClaim(c *gc.C) {
	command := raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}
	resp := s.apply(c, command)
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertClaimed(c, resp, lease.Key{"ns", "model", "lease"}, "me")

	c.Assert(s.fsm.Leases(zero), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(time.Second),
			},
		},
	)

	// Can't claim it again.
	resp = s.apply(c, command)
	c.Assert(resp.Error(), jc.Satisfies, lease.IsInvalid)
	assertNoNotifications(c, resp)

	// Someone else trying to claim the lease.
	command.Holder = "you"
	resp = s.apply(c, command)
	c.Assert(resp.Error(), jc.Satisfies, lease.IsInvalid)
	assertNoNotifications(c, resp)
}

func offset(d time.Duration) time.Time {
	return zero.Add(d)
}

func (s *fsmSuite) TestExtend(c *gc.C) {
	// Can't extend unless we've previously claimed.
	command := raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationExtend,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}
	resp := s.apply(c, command)
	c.Assert(resp.Error(), jc.Satisfies, lease.IsInvalid)
	assertNoNotifications(c, resp)

	// Ok, so we'll claim it.
	command.Operation = raftlease.OperationClaim
	resp = s.apply(c, command)
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertClaimed(c, resp, lease.Key{"ns", "model", "lease"}, "me")

	// Now we can extend it.
	command.Operation = raftlease.OperationExtend
	command.Duration = 2 * time.Second
	resp = s.apply(c, command)
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertNoNotifications(c, resp)

	c.Assert(s.fsm.Leases(zero), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(2 * time.Second),
			},
		},
	)

	// Extending by a time less than the remaining duration doesn't
	// shorten the lease (but does succeed).
	command.Duration = time.Millisecond
	resp = s.apply(c, command)
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertNoNotifications(c, resp)

	c.Assert(s.fsm.Leases(zero), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(2 * time.Second),
			},
		},
	)

	// Someone else can't extend it.
	command.Holder = "you"
	resp = s.apply(c, command)
	c.Assert(resp.Error(), jc.Satisfies, lease.IsInvalid)
	assertNoNotifications(c, resp)
}

func (s *fsmSuite) TestSetTime(c *gc.C) {
	// Time always starts at 0.
	resp := s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   zero.Add(2 * time.Second),
	})
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertNoNotifications(c, resp)
	c.Assert(s.fsm.GlobalTime(), gc.Equals, zero.Add(2*time.Second))

	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   zero.Add(time.Second),
	}).Error(), jc.Satisfies, globalclock.IsConcurrentUpdate)
}

func (s *fsmSuite) TestSetTimeExpiresLeases(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   offset(2 * time.Second),
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "much-earlier",
		Holder:    "you",
		Duration:  time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "just-before",
		Holder:    "you",
		Duration:  (2 * time.Second) - time.Nanosecond,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "bang-on",
		Holder:    "you",
		Duration:  2 * time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "well-after",
		Holder:    "them",
		Duration:  time.Minute,
	}).Error(), jc.ErrorIsNil)

	// Advance time by another 2 seconds, and two of the leases are
	// autoexpired.
	resp := s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   offset(2 * time.Second),
		NewTime:   offset(4 * time.Second),
	})
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertExpired(c, resp,
		lease.Key{"ns", "model", "much-earlier"},
		lease.Key{"ns", "model", "just-before"},
	)

	// Using the same local time as global time to keep things clear.
	c.Assert(s.fsm.Leases(offset(4*time.Second)), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "bang-on"}: {
				Holder: "you",
				Expiry: offset(4 * time.Second),
			},
			{"ns", "model", "well-after"}: {
				Holder: "them",
				Expiry: offset(62 * time.Second),
			},
		},
	)
}

func (s *fsmSuite) TestPinUnpin(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   offset(2 * time.Second),
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}).Error(), jc.ErrorIsNil)

	machineTag := names.NewMachineTag("0").String()
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationPin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: machineTag,
	}).Error(), jc.ErrorIsNil)

	// Pinned lease does not expire.
	resp := s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   offset(2 * time.Second),
		NewTime:   offset(4 * time.Second),
	})
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertExpired(c, resp)

	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationUnpin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: machineTag,
	}).Error(), jc.ErrorIsNil)

	// Unpinned lease expires when time advances.
	resp = s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   offset(4 * time.Second),
		NewTime:   offset(6 * time.Second),
	})
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertExpired(c, resp, lease.Key{"ns", "model", "lease"})
}

func (s *fsmSuite) TestPinUnpinMultipleHoldersNoExpiry(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   offset(2 * time.Second),
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}).Error(), jc.ErrorIsNil)

	// Two different entities pin the same lease.
	machineTag := names.NewMachineTag("0").String()
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationPin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: machineTag,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationPin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: names.NewMachineTag("1").String(),
	}).Error(), jc.ErrorIsNil)

	// One entity releases.
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationUnpin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: machineTag,
	}).Error(), jc.ErrorIsNil)

	// Lease does not expire, as there is still one pin.
	resp := s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   offset(2 * time.Second),
		NewTime:   offset(4 * time.Second),
	})
	c.Assert(resp.Error(), jc.ErrorIsNil)
	assertExpired(c, resp)
}

func (s *fsmSuite) TestLeases(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns2",
		ModelUUID: "model2",
		Lease:     "lease",
		Holder:    "you",
		Duration:  4 * time.Second,
	}).Error(), jc.ErrorIsNil)

	c.Assert(s.fsm.Leases(zero), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(time.Second),
			},
			{"ns2", "model2", "lease"}: {
				Holder: "you",
				Expiry: offset(4 * time.Second),
			},
		},
	)
}

func (s *fsmSuite) TestLeasesPinnedFutureExpiry(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationPin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: names.NewMachineTag("0").String(),
	}).Error(), jc.ErrorIsNil)

	// Even though the lease duration is only one second,
	// expiry should be represented as 30 seconds in the future.
	c.Assert(s.fsm.Leases(zero), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(30 * time.Second),
			},
		},
	)
}

func (s *fsmSuite) TestLeasesDifferentTime(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  5 * time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns2",
		ModelUUID: "model2",
		Lease:     "lease",
		Holder:    "you",
		Duration:  7 * time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   zero.Add(2 * time.Second),
	}).Error(), jc.ErrorIsNil)

	// Global time is 00:00:02, but we think it's only 00:00:01
	c.Assert(s.fsm.Leases(offset(time.Second)), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(4 * time.Second),
			},
			{"ns2", "model2", "lease"}: {
				Holder: "you",
				Expiry: offset(6 * time.Second),
			},
		},
	)

	// Global time is 00:00:02, but we think it's 00:00:04!
	c.Assert(s.fsm.Leases(offset(4*time.Second)), gc.DeepEquals,
		map[lease.Key]lease.Info{
			{"ns", "model", "lease"}: {
				Holder: "me",
				Expiry: offset(7 * time.Second),
			},
			{"ns2", "model2", "lease"}: {
				Holder: "you",
				Expiry: offset(9 * time.Second),
			},
		},
	)
}

func (s *fsmSuite) TestApplyInvalidCommand(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   300,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   zero.Add(2 * time.Second),
	}).Error(), jc.Satisfies, errors.IsNotValid)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: "libera-me",
	}).Error(), jc.Satisfies, errors.IsNotValid)
}

func (s *fsmSuite) TestSnapshot(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  3 * time.Second,
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   zero,
		NewTime:   zero.Add(2 * time.Second),
	}).Error(), jc.ErrorIsNil)
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns2",
		ModelUUID: "model2",
		Lease:     "lease",
		Holder:    "you",
		Duration:  4 * time.Second,
	}).Error(), jc.ErrorIsNil)

	machineTag := names.NewMachineTag("0")
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationPin,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: machineTag.String(),
	}).Error(), jc.ErrorIsNil)

	snapshot, err := s.fsm.Snapshot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(snapshot, gc.DeepEquals, &raftlease.Snapshot{
		Version: 1,
		Entries: map[raftlease.SnapshotKey]raftlease.SnapshotEntry{
			{"ns", "model", "lease"}: {
				Holder:   "me",
				Start:    zero,
				Duration: 3 * time.Second,
			},
			{"ns2", "model2", "lease"}: {
				Holder:   "you",
				Start:    zero.Add(2 * time.Second),
				Duration: 4 * time.Second,
			},
		},
		GlobalTime: zero.Add(2 * time.Second),
		Pinned: map[raftlease.SnapshotKey][]string{
			{"ns", "model", "lease"}: {machineTag.String()},
		},
	})
}

func (s *fsmSuite) TestRestore(c *gc.C) {
	c.Assert(s.apply(c, raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "ns",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "me",
		Duration:  time.Second,
	}).Error(), jc.ErrorIsNil)

	// Restoring overwrites the state.
	reader := closer{Reader: bytes.NewBuffer([]byte(snapshotYaml))}
	err := s.fsm.Restore(&reader)
	c.Assert(err, jc.ErrorIsNil)

	expected := &raftlease.Snapshot{
		Version: 1,
		Entries: map[raftlease.SnapshotKey]raftlease.SnapshotEntry{
			{"ns", "model", "lease"}: {
				Holder:   "me",
				Start:    zero,
				Duration: 5 * time.Second,
			},
			{"ns2", "model2", "lease"}: {
				Holder:   "you",
				Start:    zero.Add(2 * time.Second),
				Duration: 10 * time.Second,
			},
		},
		GlobalTime: zero.Add(3 * time.Second),
		Pinned: map[raftlease.SnapshotKey][]string{
			{"ns", "model", "lease"}: {names.NewMachineTag("0").String()},
		},
	}

	actual, err := s.fsm.Snapshot()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(actual, gc.DeepEquals, expected)
}

func (s *fsmSuite) TestSnapshotPersist(c *gc.C) {
	snapshot := &raftlease.Snapshot{
		Version: 1,
		Entries: map[raftlease.SnapshotKey]raftlease.SnapshotEntry{
			{"ns", "model", "lease"}: {
				Holder:   "me",
				Start:    zero,
				Duration: time.Second,
			},
			{"ns2", "model2", "lease"}: {
				Holder:   "you",
				Start:    zero.Add(2 * time.Second),
				Duration: 4 * time.Second,
			},
		},
		Pinned: map[raftlease.SnapshotKey][]string{
			{"ns", "model", "lease"}: {names.NewMachineTag("0").String()},
		},
		GlobalTime: zero.Add(2 * time.Second),
	}
	var buffer bytes.Buffer
	sink := fakeSnapshotSink{Writer: &buffer}
	err := snapshot.Persist(&sink)
	c.Assert(err, gc.ErrorMatches, "quam olim abrahe")
	c.Assert(sink.cancelled, gc.Equals, true)

	// Don't compare buffer bytes in output yaml directly, it's
	// dependent on map ordering.
	decoder := yaml.NewDecoder(&buffer)
	var loaded raftlease.Snapshot
	err = decoder.Decode(&loaded)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(&loaded, gc.DeepEquals, snapshot)
}

func (s *fsmSuite) TestCommandValidationClaim(c *gc.C) {
	command := raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationClaim,
		Namespace: "namespace",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "you",
		Duration:  time.Second,
	}
	c.Assert(command.Validate(), gc.Equals, nil)
	command.OldTime = time.Now()
	c.Assert(command.Validate(), gc.ErrorMatches, "claim with old time not valid")
	command.OldTime = time.Time{}
	command.Lease = ""
	c.Assert(command.Validate(), gc.ErrorMatches, "claim with empty lease not valid")
}

func (s *fsmSuite) TestCommandValidationExtend(c *gc.C) {
	command := raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationExtend,
		Namespace: "namespace",
		ModelUUID: "model",
		Lease:     "lease",
		Holder:    "you",
		Duration:  time.Second,
	}
	c.Assert(command.Validate(), gc.Equals, nil)
	command.NewTime = time.Now()
	c.Assert(command.Validate(), gc.ErrorMatches, "extend with new time not valid")
	command.OldTime = time.Time{}
	command.Namespace = ""
	c.Assert(command.Validate(), gc.ErrorMatches, "extend with empty namespace not valid")
}

func (s *fsmSuite) TestCommandValidationSetTime(c *gc.C) {
	command := raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationSetTime,
		OldTime:   time.Now(),
		NewTime:   time.Now(),
	}
	c.Assert(command.Validate(), gc.Equals, nil)
	command.Duration = time.Minute
	c.Assert(command.Validate(), gc.ErrorMatches, "setTime with duration not valid")
	command.Duration = 0
	command.NewTime = time.Time{}
	c.Assert(command.Validate(), gc.ErrorMatches, "setTime with zero new time not valid")
}

func (s *fsmSuite) TestCommandValidationPin(c *gc.C) {
	command := raftlease.Command{
		Version:   1,
		Operation: raftlease.OperationPin,
		Namespace: "namespace",
		ModelUUID: "model",
		Lease:     "lease",
		PinEntity: names.NewMachineTag("0").String(),
	}
	c.Assert(command.Validate(), gc.Equals, nil)
	command.NewTime = time.Now()
	c.Assert(command.Validate(), gc.ErrorMatches, "pin with new time not valid")
	command.NewTime = time.Time{}
	command.Namespace = ""
	c.Assert(command.Validate(), gc.ErrorMatches, "pin with empty namespace not valid")
	command.Namespace = "namespace"
	command.Duration = time.Minute
	c.Assert(command.Validate(), gc.ErrorMatches, "pin with duration not valid")
	command.Duration = 0
	command.PinEntity = ""
	c.Assert(command.Validate(), gc.ErrorMatches, "pin with empty pin entity not valid")
}

func assertClaimed(c *gc.C, resp raftlease.FSMResponse, key lease.Key, holder string) {
	var target fakeTarget
	resp.Notify(&target)
	c.Assert(target.Calls(), gc.HasLen, 1)
	target.CheckCall(c, 0, "Claimed", key, holder)
}

func assertExpired(c *gc.C, resp raftlease.FSMResponse, keys ...lease.Key) {
	// Don't assume the keys are expired in the order given.
	keySet := make(map[lease.Key]bool, len(keys))
	for _, key := range keys {
		keySet[key] = true
	}
	var target fakeTarget
	resp.Notify(&target)
	c.Assert(target.Calls(), gc.HasLen, len(keys))
	for _, call := range target.Calls() {
		c.Assert(call.FuncName, gc.Equals, "Expired")
		c.Assert(call.Args, gc.HasLen, 1)
		key, ok := call.Args[0].(lease.Key)
		c.Assert(ok, gc.Equals, true)
		_, found := keySet[key]
		c.Assert(found, gc.Equals, true)
		delete(keySet, key)
	}
}

func assertNoNotifications(c *gc.C, resp raftlease.FSMResponse) {
	var target fakeTarget
	resp.Notify(&target)
	c.Assert(target.Calls(), gc.HasLen, 0)
}

type fakeTarget struct {
	testing.Stub
}

func (t *fakeTarget) Claimed(key lease.Key, holder string) {
	t.AddCall("Claimed", key, holder)
}

func (t *fakeTarget) Expired(key lease.Key) {
	t.AddCall("Expired", key)
}

type fakeSnapshotSink struct {
	io.Writer
	cancelled bool
}

func (s *fakeSnapshotSink) ID() string {
	return "fakeSink"
}

func (s *fakeSnapshotSink) Cancel() error {
	s.cancelled = true
	return nil
}

func (s *fakeSnapshotSink) Close() error {
	return errors.Errorf("quam olim abrahe")
}

type closer struct {
	io.Reader
	closed bool
}

func (c *closer) Close() error {
	c.closed = true
	return nil
}

var snapshotYaml = `
version: 1
entries:
  ? namespace: ns
    model-uuid: model
    lease: lease
  : holder: me
    start: 0001-01-01T00:00:00Z
    duration: 5s
  ? namespace: ns2
    model-uuid: model2
    lease: lease
  : holder: you
    start: 0001-01-01T00:00:02Z
    duration: 10s
global-time: 0001-01-01T00:00:03Z
pinned:
  ? namespace: ns
    model-uuid: model
    lease: lease
  : [machine-0]
`[1:]
