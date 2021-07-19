// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"fmt"
	"io"
	"sort"
)

type PubsubRule struct {
	found   map[string]bool
	targets map[string][]string
}

func NewPubsubRule() *PubsubRule {
	return &PubsubRule{
		found:   make(map[string]bool),
		targets: make(map[string][]string),
	}
}

func (r *PubsubRule) Run(name string, report Report) error {
	pubsub, ok := report.Manifolds["pubsub-forwarder"]
	if !ok {
		r.found[name] = false
		return nil
	}

	r.found[name] = true

	var out PubsubReport
	if err := pubsub.UnmarshalReport(&out); err != nil {
		return err
	}

	var targets []string
	for name, target := range out.Targets {
		if target.Status == "connected" {
			targets = append(targets, name)
		}
	}
	r.targets[name] = targets
	return nil
}

func (r *PubsubRule) Write(w io.Writer) {
	fmt.Fprintln(w, "Pubsub Forwarder:")
	fmt.Fprintln(w, "")
	for name, targets := range r.targets {
		if !r.found[name] {
			fmt.Fprintf(w, "\t%s pubsub-forwarder not found!\n", name)
			continue
		}
		sort.Strings(targets)
		fmt.Fprintf(w, "\t%s is connected to the following: %v\n", name, targets)
	}
	fmt.Fprintln(w, "")
}

type PubsubReport struct {
	Targets map[string]PubsubTarget `yaml:"targets"`
}

type PubsubTarget struct {
	Status string
}
