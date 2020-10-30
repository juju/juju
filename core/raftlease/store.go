// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/pubsub"
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

// ReadonlyFSM defines the methods of the lease FSM the store can use
// - any writes must go through the hub.
type ReadonlyFSM interface {
	// Leases and LeaseGroup receive a func for retrieving time,
	// because it needs to be determined after potential lock-waiting
	// to be accurate.
	Leases(func() time.Time, ...lease.Key) map[lease.Key]lease.Info
	LeaseGroup(func() time.Time, string, string) map[lease.Key]lease.Info
	GlobalTime() time.Time
	Pinned() map[lease.Key][]string
}

// StoreConfig holds resources and settings needed to run the Store.
type StoreConfig struct {
	FSM           ReadonlyFSM
	Hub           *pubsub.StructuredHub
	Trapdoor      TrapdoorFunc
	RequestTopic  string
	ResponseTopic func(requestID uint64) string

	Clock          clock.Clock
	ForwardTimeout time.Duration
}

// NewStore returns a core/lease.Store that manages leases in Raft.
func NewStore(config StoreConfig) *Store {
	return &Store{
		fsm:      config.FSM,
		hub:      config.Hub,
		config:   config,
		prevTime: config.FSM.GlobalTime(),
		metrics:  newMetricsCollector(),
	}
}

// Store manages a raft FSM and forwards writes through a pubsub hub.
type Store struct {
	fsm       ReadonlyFSM
	hub       *pubsub.StructuredHub
	requestID uint64
	config    StoreConfig
	metrics   *metricsCollector

	prevTimeMu sync.Mutex
	prevTime   time.Time
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

// Refresh is part of lease.Store.
func (s *Store) Refresh() error {
	return nil
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

// Advance is part of globalclock.Updater.
func (s *Store) Advance(duration time.Duration, stop <-chan struct{}) error {
	s.prevTimeMu.Lock()
	defer s.prevTimeMu.Unlock()
	newTime := s.prevTime.Add(duration)
	err := s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationSetTime,
		OldTime:   s.prevTime,
		NewTime:   newTime,
	}, stop)
	if globalclock.IsConcurrentUpdate(err) {
		// Someone else updated before us - get the new time.
		s.prevTime = s.fsm.GlobalTime()
	} else if lease.IsTimeout(err) {
		// Convert this to a globalclock timeout to match the Updater
		// interface.
		err = globalclock.ErrTimeout
	} else if err == nil {
		s.prevTime = newTime
	}
	return errors.Trace(err)
}

func (s *Store) runOnLeader(command *Command, stop <-chan struct{}) error {
	bytes, err := command.Marshal()
	if err != nil {
		return errors.Trace(err)
	}
	requestID := atomic.AddUint64(&s.requestID, 1)
	responseTopic := s.config.ResponseTopic(requestID)

	responseChan := make(chan ForwardResponse, 1)
	errChan := make(chan error)
	unsubscribe, err := s.hub.Subscribe(
		responseTopic,
		func(_ string, resp ForwardResponse, err error) {
			if err != nil {
				errChan <- err
				return
			}
			responseChan <- resp
		},
	)
	if err != nil {
		return errors.Annotatef(err, "running %s", command)
	}
	defer unsubscribe()

	start := time.Now()
	defer func() {
		elapsed := time.Now().Sub(start)
		logger.Tracef("runOnLeader %v, elapsed from publish: %v", command.Operation, elapsed.Round(time.Millisecond))
	}()
	_, err = s.hub.Publish(s.config.RequestTopic, ForwardRequest{
		Command:       string(bytes),
		ResponseTopic: responseTopic,
	})
	if err != nil {
		s.record(command.Operation, "error", start)
		return errors.Annotatef(err, "publishing %s", command)
	}

	select {
	case <-s.config.Clock.After(s.config.ForwardTimeout):
		// TODO (thumper) 2019-12-20, bug 1857072
		// Scale testing hit this a *lot*,
		// perhaps we need to consider batching messages to run on the leader?
		logger.Errorf("timeout waiting for %s to be processed", command)
		s.record(command.Operation, "timeout", start)
		return lease.ErrTimeout
	case err := <-errChan:
		logger.Errorf("processing %s: %v", command, err)
		s.record(command.Operation, "error", start)
		return errors.Trace(err)
	case response := <-responseChan:
		err := RecoverError(response.Error)
		logger.Tracef("got response, err %v", err)
		result := "success"
		if err != nil {
			logger.Errorf("command %s: %v", command, err)
			result = "failure"
		}
		s.record(command.Operation, result, start)
		return err
	case <-stop:
		return aborted(command)
	}
}

func (s *Store) record(operation, result string, start time.Time) {
	elapsedMS := float64(time.Now().Sub(start)) / float64(time.Millisecond)
	s.metrics.requests.With(prometheus.Labels{
		"operation": operation,
		"result":    result,
	}).Observe(elapsedMS)
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
	case globalclock.ErrConcurrentUpdate:
		code = "concurrent-update"
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
	case "concurrent-update":
		return globalclock.ErrConcurrentUpdate
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
