// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"bytes"
	"fmt"
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

func (r *PubsubRule) Run(name string, report Report) {
	pubsub, ok := report.Manifolds["pubsub-forwarder"]
	if !ok {
		r.found[name] = false
		return
	}

	r.found[name] = true

	var targets []string
	for name, target := range pubsub.Report.Targets {
		if target.Status == "connected" {
			targets = append(targets, name)
		}
	}
	r.targets[name] = targets
}

func (r *PubsubRule) Summary() string {
	return "Pubsub Forwarder:"
}

func (r *PubsubRule) Analyse() string {
	buf := new(bytes.Buffer)
	for name, targets := range r.targets {
		if !r.found[name] {
			fmt.Fprintf(buf, "%s pubsub-forwarder not found!\n", name)
			continue
		}
		sort.Strings(targets)
		fmt.Fprintf(buf, "%s is connected to the following: %v\n", name, targets)
	}
	return buf.String()
}
