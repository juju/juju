// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftlease

import (
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
	Leases(time.Time) map[lease.Key]lease.Info
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
func (s *Store) ClaimLease(key lease.Key, req lease.Request) error {
	err := s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationClaim,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    req.Holder,
		Duration:  req.Duration,
	})
	return errors.Trace(err)
}

// ExtendLease is part of lease.Store.
func (s *Store) ExtendLease(key lease.Key, req lease.Request) error {
	return errors.Trace(s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationExtend,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		Holder:    req.Holder,
		Duration:  req.Duration,
	}))
}

// ExpireLease is part of lease.Store.
func (s *Store) ExpireLease(key lease.Key) error {
	// It's always an invalid operation - expiration happens
	// automatically when time is advanced.
	return lease.ErrInvalid
}

// Leases is part of lease.Store.
func (s *Store) Leases() map[lease.Key]lease.Info {
	leaseMap := s.fsm.Leases(s.config.Clock.Now())
	result := make(map[lease.Key]lease.Info, len(leaseMap))
	// Add trapdoors into the information from the FSM.
	for k, v := range leaseMap {
		v.Trapdoor = s.config.Trapdoor(k, v.Holder)
		result[k] = v
	}
	return result
}

// Refresh is part of lease.Store.
func (s *Store) Refresh() error {
	return nil
}

// PinLease is part of lease.Store.
func (s *Store) PinLease(key lease.Key, entity string) error {
	return errors.Trace(s.pinOp(OperationPin, key, entity))
}

// UnpinLease is part of lease.Store.
func (s *Store) UnpinLease(key lease.Key, entity string) error {
	return errors.Trace(s.pinOp(OperationUnpin, key, entity))
}

// Pinned is part of the Store interface.
func (s *Store) Pinned() map[lease.Key][]string {
	return s.fsm.Pinned()
}

func (s *Store) pinOp(operation string, key lease.Key, entity string) error {
	return errors.Trace(s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: operation,
		Namespace: key.Namespace,
		ModelUUID: key.ModelUUID,
		Lease:     key.Lease,
		PinEntity: entity,
	}))
}

// Advance is part of globalclock.Updater.
func (s *Store) Advance(duration time.Duration) error {
	s.prevTimeMu.Lock()
	defer s.prevTimeMu.Unlock()
	newTime := s.prevTime.Add(duration)
	err := s.runOnLeader(&Command{
		Version:   CommandVersion,
		Operation: OperationSetTime,
		OldTime:   s.prevTime,
		NewTime:   newTime,
	})
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

func (s *Store) runOnLeader(command *Command) error {
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
		return errors.Trace(err)
	}
	defer unsubscribe()

	start := time.Now()
	defer func() {
		elapsed := time.Now().Sub(start)
		logger.Tracef("runOnLeader elapsed from publish: %v", elapsed.Round(time.Millisecond))
	}()
	_, err = s.hub.Publish(s.config.RequestTopic, ForwardRequest{
		Command:       string(bytes),
		ResponseTopic: responseTopic,
	})
	if err != nil {
		s.record(command.Operation, "error", start)
		return errors.Trace(err)
	}

	select {
	case <-s.config.Clock.After(s.config.ForwardTimeout):
		logger.Infof("timeout")
		s.record(command.Operation, "timeout", start)
		return lease.ErrTimeout
	case err := <-errChan:
		logger.Errorf("%v", err)
		s.record(command.Operation, "error", start)
		return errors.Trace(err)
	case response := <-responseChan:
		err := RecoverError(response.Error)
		logger.Tracef("got response, err %v", err)
		result := "failure"
		if err == nil {
			result = "success"
		}
		s.record(command.Operation, result, start)
		return err
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
