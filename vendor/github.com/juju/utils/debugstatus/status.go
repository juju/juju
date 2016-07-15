// Copyright 2014 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

// Package debugstatus provides facilities for inspecting information
// about a running HTTP service.
package debugstatus

import (
	"fmt"
	"sync"
	"time"

	"gopkg.in/mgo.v2"
)

// Check collects the status check results from the given checkers.
func Check(checkers ...CheckerFunc) map[string]CheckResult {
	var mu sync.Mutex
	results := make(map[string]CheckResult, len(checkers))

	var wg sync.WaitGroup
	for _, c := range checkers {
		c := c
		wg.Add(1)
		go func() {
			defer wg.Done()
			t0 := time.Now()
			key, result := c()
			result.Duration = time.Since(t0)
			mu.Lock()
			results[key] = result
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

// CheckResult holds the result of a single status check.
type CheckResult struct {
	// Name is the human readable name for the check.
	Name string

	// Value is the check result.
	Value string

	// Passed reports whether the check passed.
	Passed bool

	// Duration holds the duration that the
	// status check took to run.
	Duration time.Duration
}

// CheckerFunc represents a function returning the check machine friendly key
// and the result.
type CheckerFunc func() (key string, result CheckResult)

// StartTime holds the time that the code started running.
var StartTime = time.Now().UTC()

// ServerStartTime reports the time when the application was started.
func ServerStartTime() (key string, result CheckResult) {
	return "server_started", CheckResult{
		Name:   "Server started",
		Value:  StartTime.String(),
		Passed: true,
	}
}

// Connection returns a status checker reporting whether the given Pinger is
// connected.
func Connection(p Pinger) CheckerFunc {
	return func() (key string, result CheckResult) {
		result.Name = "MongoDB is connected"
		if err := p.Ping(); err != nil {
			result.Value = "Ping error: " + err.Error()
			return "mongo_connected", result
		}
		result.Value = "Connected"
		result.Passed = true
		return "mongo_connected", result
	}
}

// Pinger is an interface that wraps the Ping method.
// It is implemented by mgo.Session.
type Pinger interface {
	Ping() error
}

var _ Pinger = (*mgo.Session)(nil)

// MongoCollections returns a status checker checking that all the
// expected Mongo collections are present in the database.
func MongoCollections(c Collector) CheckerFunc {
	return func() (key string, result CheckResult) {
		key = "mongo_collections"
		result.Name = "MongoDB collections"
		names, err := c.CollectionNames()
		if err != nil {
			result.Value = "Cannot get collections: " + err.Error()
			return key, result
		}
		var missing []string
		for _, coll := range c.Collections() {
			found := false
			for _, name := range names {
				if name == coll.Name {
					found = true
					break
				}
			}
			if !found {
				missing = append(missing, coll.Name)
			}
		}
		if len(missing) == 0 {
			result.Value = "All required collections exist"
			result.Passed = true
			return key, result
		}
		result.Value = fmt.Sprintf("Missing collections: %s", missing)
		return key, result
	}
}

// Collector is an interface that groups the methods used to check that
// a Mongo database has the expected collections.
// It is usually implemented by types extending mgo.Database to add the
// Collections() method.
type Collector interface {
	// Collections returns the Mongo collections that we expect to exist in
	// the Mongo database.
	Collections() []*mgo.Collection

	// CollectionNames returns the names of the collections actually present in
	// the Mongo database.
	CollectionNames() ([]string, error)
}

// Rename changes the key and/or result name returned by the given check.
// It is possible to pass an empty string to avoid changing one of the values.
// This means that if both key are name are empty, this closure is a no-op.
func Rename(newKey, newName string, check CheckerFunc) CheckerFunc {
	return func() (key string, result CheckResult) {
		key, result = check()
		if newKey == "" {
			newKey = key
		}
		if newName != "" {
			result.Name = newName
		}
		return newKey, result
	}
}
