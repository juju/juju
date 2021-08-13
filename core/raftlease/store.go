// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"context"
	"fmt"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/juju/juju/core/globalclock"
	"github.com/juju/juju/core/lease"
)

var logger = loggo.GetLogger("juju.core.raftlease")

func aborted(command *Command) error {
	switch command.Operation {
	case OperationSetTime:
		return errors.Annotatef(lease.ErrAborted, "setTime")
	case OperationPin, OperationUnpin:
		leaseId := fmt.Sprintf("%.6s:%s", command.ModelUUID, command.Lease)
		return errors.Annotatef(lease.ErrAborted, "%q on %q",
			command.Operation, leaseId)
	default:
		leaseId := fmt.Sprintf("%.6s:%s", command.ModelUUID, command.Lease)
		return errors.Annotatef(lease.ErrAborted, "%q on %q for %q",
			command.Operation, leaseId, command.Holder)
	}
}

// NotifyTarget defines methods needed to keep an external database
// updated with who holds leases. (In non-test code the notify target
// will generally be the state DB.)
type NotifyTarget interface {
	// Claimed will be called when a new lease has been claimed. Not
	// allowed to return an error because this is purely advisory -
	// the lease claim has still occurred, whether or not the callback
	// succeeds.
	Claimed(lease.Key, string)

	// Expired will be called when an existing lease has expired. Not
	// allowed to return an error because this is purely advisory.
	Expired(lease.Key)
}

// TrapdoorFunc returns a trapdoor to be attached to lease details for
// use by clients. This is intended to hold assertions that can be
// added to state transactions to ensure the lease is still held when
// the transaction is applied.
type TrapdoorFunc func(lease.Key, string) lease.Trapdoor

// ReadOnlyClock describes a clock from which global time can be read.
type ReadOnlyClock interface {
	GlobalTime() time.Time
}

// ReadonlyFSM defines the methods of the lease FSM the store can use
// - any writes must go through the hub.
type ReadonlyFSM interface {
	ReadOnlyClock

	// Leases and LeaseGroup receive a func for retrieving time,
	// because it needs to be determined after potential lock-waiting
	// to be accurate.
	Leases(func() time.Time, ...lease.Key) map[lease.Key]lease.Info
	LeaseGroup(func() time.Time, string, string) map[lease.Key]lease.Info
	Pinned() map[lease.Key][]string
}

// StoreConfig holds resources and settings needed to run the Store.
type StoreConfig struct {
	FSM              ReadonlyFSM
	Client           Client
	Trapdoor         TrapdoorFunc
	Clock            clock.Clock
	MetricsCollector MetricsCollector
}

// Store manages a raft FSM and forwards writes through a pubsub hub.
type Store struct {
	fsm     ReadonlyFSM
	config  StoreConfig
	metrics MetricsCollector
	client  Client
}

// NewStore returns a core/lease.Store that manages leases in Raft.
func NewStore(config StoreConfig) *Store {
	return &Store{
		fsm:     config.FSM,
		config:  config,
		client:  config.Client,
		metrics: config.MetricsCollector,
	}
}

// ClaimLease is part of lease.Store.
func (s *Store) ClaimLease(key lease.Key, req lease.Request, stop <-chan struct{}) error {
	return errors.Trace(s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationClaim,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    req.Holder,
		Duration:  req.Duration,
	}, stop))
}

// ExtendLease is part of lease.Store.
func (s *Store) ExtendLease(key lease.Key, req lease.Request, stop <-chan struct{}) error {
	return errors.Trace(s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationExtend,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    req.Holder,
		Duration:  req.Duration,
	}, stop))
}

// RevokeLease is part of lease.Store.
func (s *Store) RevokeLease(key lease.Key, holder string, stop <-chan struct{}) error {
	return errors.Trace(s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationRevoke,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    holder,
	}, stop))
}

// Leases is part of lease.Store.
func (s *Store) Leases(keys ...lease.Key) map[lease.Key]lease.Info {
	leaseMap := s.fsm.Leases(s.config.Clock.Now, keys...)
	s.addTrapdoors(leaseMap)
	return leaseMap
}

// LeaseGroup is part of Lease.Store.
func (s *Store) LeaseGroup(namespace, modelUUID string) map[lease.Key]lease.Info {
	leaseMap := s.fsm.LeaseGroup(s.config.Clock.Now, namespace, modelUUID)
	s.addTrapdoors(leaseMap)
	return leaseMap
}

func (s *Store) addTrapdoors(leaseMap map[lease.Key]lease.Info) {
	for k, v := range leaseMap {
		v.Trapdoor = s.config.Trapdoor(k, v.Holder)
		leaseMap[k] = v
	}
}

// PinLease is part of lease.Store.
func (s *Store) PinLease(key lease.Key, entity string, stop <-chan struct{}) error {
	return errors.Trace(s.pinOp(OperationPin, key, entity, stop))
}

// UnpinLease is part of lease.Store.
func (s *Store) UnpinLease(key lease.Key, entity string, stop <-chan struct{}) error {
	return errors.Trace(s.pinOp(OperationUnpin, key, entity, stop))
}

// Pinned is part of the Store interface.
func (s *Store) Pinned() map[lease.Key][]string {
	return s.fsm.Pinned()
}

// Close the store and any underlying clients.
func (s *Store) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

func (s *Store) pinOp(operation string, key lease.Key, entity string, stop <-chan struct{}) error {
	return errors.Trace(s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: operation,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		PinEntity: entity,
	}, stop))
}

func (s *Store) runOnLeader(command *Command, stop <-chan struct{}) error {
	start := s.config.Clock.Now()
	defer func() {
		elapsed := s.config.Clock.Now().Sub(start)
		logger.Tracef("runOnLeader %v, elapsed from publish: %v", command.Operation, elapsed.Round(time.Millisecond))
	}()

	ch := make(chan struct{})
	defer close(ch)

	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()
	go func() {
		select {
		case <-stop:
			cancel()
		case <-ch:
		}
	}()

	return s.client.Request(ctx, command)
}

// ForwardRequest is a message sent over the hub to the raft forwarder
// (only running on the raft leader node).
type ForwardRequest struct {
	Command       string `yaml:"command"`
	ResponseTopic string `yaml:"response-topic"`
}

// ForwardResponse is the response sent back from the raft forwarder.
type ForwardResponse struct {
	Error *ResponseError `yaml:"error"`
}

// ResponseError is used for sending error values back to the lease
// store via the hub.
type ResponseError struct {
	Message string `yaml:"message"`
	Code    string `yaml:"code"`
}

// AsResponseError returns a *ResponseError that can be sent back over
// the hub in response to a forwarded FSM command.
func AsResponseError(err error) *ResponseError {
	if err == nil {
		return nil
	}
	message := err.Error()
	var code string
	switch errors.Cause(err) {
	case lease.ErrInvalid:
		code = "invalid"
	case globalclock.ErrOutOfSyncUpdate:
		code = "out-of-sync"
	case lease.ErrHeld:
		code = "already-held"
	default:
		code = "error"
	}
	return &ResponseError{
		Message: message,
		Code:    code,
	}
}

// RecoverError converts a ResponseError back into the specific error
// it represents, or into a generic error if it wasn't one of the
// singleton errors handled.
func RecoverError(resp *ResponseError) error {
	if resp == nil {
		return nil
	}
	switch resp.Code {
	case "invalid":
		return lease.ErrInvalid
	case "out-of-sync":
		return globalclock.ErrOutOfSyncUpdate
	case "already-held":
		return lease.ErrHeld
	default:
		return errors.New(resp.Message)
	}
}

// Describe is part of prometheus.Collector.
func (s *Store) Describe(ch chan<- *prometheus.Desc) {
	s.metrics.Describe(ch)
}

// Collect is part of prometheus.Collector.
func (s *Store) Collect(ch chan<- prometheus.Metric) {
	s.metrics.Collect(ch)
}
