// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import "fmt"

type MongoRule struct {
	found     map[string]bool
	primaries map[string]bool
}

func NewMongoRule() *MongoRule {
	return &MongoRule{
		found:     make(map[string]bool),
		primaries: make(map[string]bool),
	}
}

func (r *MongoRule) Run(name string, report Report) {
	mgo, ok := report.Manifolds["transaction-pruner"]
	if !ok {
		r.found[name] = false
		return
	}

	r.found[name] = true
	r.primaries[name] = mgo.State == "started"
}

func (r *MongoRule) Summary() string {
	return "Mongo Primary:"
}

func (r *MongoRule) Analyse() string {
	var primary bool
	var ctrl string
	for name, ldr := range r.primaries {
		if primary && ldr {
			// Two or more primaries
			return "Two or more primaries have been found in the files!"
		}
		if ldr {
			primary = true
			ctrl = name
		}
	}
	if !primary {
		return "There are no primaries found."
	}
	return fmt.Sprintf("%s is the primary.", ctrl)
}
