// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"reflect"
	"strings"
	"time"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/retry"

	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/rpc"
)

// EventType represents what type of event is being passed.
type EventType int

const (
	// WatchAllStarted defines if a watcher has started.
	WatchAllStarted EventType = iota
)

// Callback represents a way to subscribe to a given event and be called for
// all events and up to the implementation to filter for a given event.
type Callback = func(EventType)

// StrategyFunc defines a way to change the underlying stategy function that
// can be changed depending on the callee.
type StrategyFunc func(string, []params.Delta, query.Query) (bool, error)

// Strategy defines a series of instructions to run for a given wait for
// plan.
type Strategy struct {
	ClientFn    func() (api.WatchAllAPI, error)
	Timeout     time.Duration
	subscribers []Callback
}

// Subscribe a subscriber to an events coming out of the strategy.
func (s *Strategy) Subscribe(sub Callback) {
	s.subscribers = append(s.subscribers, sub)
}

// Run the strategy and return the given result set.
func (s *Strategy) Run(name string, input string, fn StrategyFunc) error {
	q, err := query.Parse(input)
	if err != nil {
		return errors.Trace(err)
	}

	return retry.Call(retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       5 * time.Second,
		MaxDuration: s.Timeout,
		Func: func() error {
			s.dispatch(WatchAllStarted)
			return s.run(q, name, input, fn)
		},
		IsFatalError: func(err error) bool {
			if e, ok := errors.Cause(err).(*rpc.RequestError); ok && isWatcherStopped(e) {
				return false
			}
			return true
		},
	})
}

func (s *Strategy) run(q query.Query, name string, input string, fn StrategyFunc) error {
	client, err := s.ClientFn()
	if err != nil {
		return errors.Trace(err)
	}

	watcher, err := client.WatchAll()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = watcher.Stop()
	}()

	timeout := make(chan struct{})
	go func() {
		select {
		case <-time.After(s.Timeout):
			close(timeout)
			_ = watcher.Stop()
		}
	}()

	for {
		deltas, err := watcher.Next()
		if err != nil {
			select {
			case <-timeout:
				return errors.Errorf("timed out waiting for %q to reach goal state", name)
			default:
				return errors.Trace(err)
			}
		}

		if done, err := fn(name, deltas, q); err != nil {
			return errors.Trace(err)
		} else if done {
			return nil
		}
	}
}

func (s *Strategy) dispatch(event EventType) {
	for _, fn := range s.subscribers {
		fn(event)
	}
}

func isWatcherStopped(e *rpc.RequestError) bool {
	// Currently multiwatcher.ErrStopped doesn't expose an error code or
	// underlying error to know if a watcher is stopped or not.
	return strings.Contains(e.Message, "watcher was stopped")
}

// GenericScope allows the query to introspect an entity.
type GenericScope struct {
	Info params.EntityInfo
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m GenericScope) GetIdentValue(name string) (query.Ord, error) {
	refType := reflect.TypeOf(m.Info).Elem()
	for i := 0; i < refType.NumField(); i++ {
		field := refType.Field(i)
		v := strings.Split(field.Tag.Get("json"), ",")[0]
		if v == name {
			refValue := reflect.ValueOf(m.Info).Elem()
			fieldValue := refValue.Field(i)
			data := fieldValue.Interface()
			switch fieldValue.Kind() {
			case reflect.Int:
				return query.NewInteger(int64(data.(int))), nil
			case reflect.Int64:
				return query.NewInteger(data.(int64)), nil
			case reflect.Float64:
				return query.NewFloat(float64(data.(float64))), nil
			case reflect.String:
				return query.NewString(data.(string)), nil
			case reflect.Bool:
				return query.NewBool(data.(bool)), nil
			}

			return nil, errors.Errorf("Runtime Error: unhandled identifier type %q for %q", refValue.Kind(), name)
		}
	}
	return nil, errors.Errorf("Runtime Error: identifier %q not found on Info", name)
}

// GetIdents returns the identifers that are supported for a given scope.
func (m GenericScope) GetIdents() []string {
	var res []string

	refType := reflect.TypeOf(m.Info).Elem()
	for i := 0; i < refType.NumField(); i++ {
		field := refType.Field(i)
		v := strings.Split(field.Tag.Get("json"), ",")[0]
		refValue := reflect.ValueOf(m.Info).Elem()

		switch refValue.Field(i).Kind() {
		case reflect.Int, reflect.Int64, reflect.Float64, reflect.String, reflect.Bool:
			res = append(res, v)
		}
	}
	return res
}
