// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"bytes"
	"fmt"
	"sort"
)

type StartCountRule struct {
	counts map[string]map[string]int
}

func NewStartCountRule() *StartCountRule {
	return &StartCountRule{
		counts: make(map[string]map[string]int),
	}
}

func (r *StartCountRule) Run(name string, report Report) {
	for manifoldName, manifold := range report.Manifolds {
		if _, ok := r.counts[name]; !ok {
			r.counts[name] = make(map[string]int)
		}
		r.counts[name][manifoldName] = manifold.StartCount
	}
}

func (r *StartCountRule) Summary() string {
	return "Start Counts:"
}

type namedCount struct {
	name  string
	count int
}

func (r *StartCountRule) Analyse() string {
	// Gather
	total := make(map[string]int, len(r.counts))
	highest := make(map[string]namedCount, len(r.counts))
	for ctrl, manifolds := range r.counts {
		for name, v := range manifolds {
			total[ctrl] += v

			nc := highest[ctrl]
			if v > nc.count {
				highest[ctrl] = namedCount{
					name:  name,
					count: v,
				}
			}
		}
	}

	order := make([]string, 0, len(total))
	for k := range total {
		order = append(order, k)
	}
	sort.Strings(order)

	// Report
	buf := new(bytes.Buffer)
	for _, ctrl := range order {
		t := total[ctrl]
		fmt.Fprintf(buf, "%s start-count: %d\n", ctrl, t)
		fmt.Fprintf(buf, "  - max: %q with: %d\n", highest[ctrl].name, highest[ctrl].count)
	}
	return buf.String()
}
