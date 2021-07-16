// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import "fmt"

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

func (r *RaftRule) Run(name string, report Report) {
	raft, ok := report.Manifolds["raft"]
	if !ok {
		r.found[name] = false
		return
	}

	r.found[name] = true
	r.leaders[name] = raft.Report.State == "Leader"
}

func (r *RaftRule) Summary() string {
	return "Raft Leader:"
}

func (r *RaftRule) Analyse() string {
	var leader bool
	var ctrl string
	for name, ldr := range r.leaders {
		if leader && ldr {
			// Two or more leaders
			return "Two or more leaders have been found in the files!"
		}
		if ldr {
			leader = true
			ctrl = name
		}
	}
	if !leader {
		return "There are no leaders found."
	}
	return fmt.Sprintf("%s is the leader.", ctrl)
}
