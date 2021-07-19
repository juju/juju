// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"fmt"
	"io"
)

type RaftRule struct {
	found   map[string]bool
	leaders map[string]bool
}

func NewRaftRule() *RaftRule {
	return &RaftRule{
		found:   make(map[string]bool),
		leaders: make(map[string]bool),
	}
}

func (r *RaftRule) Run(name string, report Report) error {
	raft, ok := report.Manifolds["raft"]
	if !ok {
		r.found[name] = false
		return nil
	}

	r.found[name] = true

	var out RaftReport
	if err := raft.UnmarshalReport(&out); err != nil {
		return err
	}

	r.leaders[name] = out.State == "Leader"

	return nil
}

func (r *RaftRule) Write(w io.Writer) {
	fmt.Fprintln(w, "Raft Leader:")
	fmt.Fprintln(w, "")

	var leader bool
	var ctrl string
	for name, ldr := range r.leaders {
		if leader && ldr {
			// Two or more leaders
			fmt.Fprintln(w, "\tTwo or more leaders have been found in the files!")
			return
		}
		if ldr {
			leader = true
			ctrl = name
		}
	}
	if !leader {
		fmt.Fprintln(w, "\tThere are no leaders found.")
		return
	}
	fmt.Fprintf(w, "\t%s is the leader.\n", ctrl)
	fmt.Fprintln(w, "")
}

type RaftReport struct {
	State string `yaml:"state"`
}
