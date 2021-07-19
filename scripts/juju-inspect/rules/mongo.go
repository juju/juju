// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"fmt"
	"io"
)

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

func (r *MongoRule) Run(name string, report Report) error {
	mgo, ok := report.Manifolds["transaction-pruner"]
	if !ok {
		r.found[name] = false
		return nil
	}

	r.found[name] = true
	r.primaries[name] = mgo.State == "started"

	return nil
}

func (r *MongoRule) Write(w io.Writer) {
	fmt.Fprintln(w, "Mongo Primary:")
	fmt.Fprintln(w, "")

	var primary bool
	var ctrl string
	for name, ldr := range r.primaries {
		if primary && ldr {
			// Two or more primaries
			fmt.Fprintln(w, "\tTwo or more primaries have been found in the files!")
			return
		}
		if ldr {
			primary = true
			ctrl = name
		}
	}
	if !primary {
		fmt.Fprintln(w, "\tThere are no primaries found.")
		return
	}
	fmt.Fprintf(w, "\t%s is the primary.\n", ctrl)
	fmt.Fprintln(w, "")
}
