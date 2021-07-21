// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"fmt"
	"io"
	"sort"
)

type StartCountRule struct {
	includeNested  bool
	highestListNum int
	counts         map[string]map[string]int
}

func NewStartCountRule(includeNested bool, highestListNum int) *StartCountRule {
	return &StartCountRule{
		includeNested:  includeNested,
		highestListNum: highestListNum,
		counts:         make(map[string]map[string]int),
	}
}

func (r *StartCountRule) Run(name string, report Report) error {
	for manifoldName, manifold := range report.Manifolds {
		if _, ok := r.counts[name]; !ok {
			r.counts[name] = make(map[string]int)
		}
		r.counts[name][manifoldName] += manifold.StartCount
	}

	if !r.includeNested {
		return nil
	}

	manager, ok := report.Manifolds["model-worker-manager"]
	if !ok {
		return nil
	}

	var nested NestedReport
	if err := manager.UnmarshalReport(&nested); err != nil {
		return err
	}

	for subName, worker := range nested.Workers {
		var nestedReport Report
		if err := worker.UnmarshalReport(&nestedReport); err != nil {
			return err
		}

		if err := r.Run(fmt.Sprintf("%s:%s", name, subName), nestedReport); err != nil {
			return err
		}
	}

	return nil
}

type namedCount struct {
	name  string
	count int
}

func (r *StartCountRule) Write(w io.Writer) {
	fmt.Fprintln(w, "Start Counts:")
	fmt.Fprintln(w, "")

	// Gather
	total := make(map[string]int, len(r.counts))
	highest := make(map[string][]namedCount, len(r.counts))
	for ctrl, manifolds := range r.counts {
		for name, v := range manifolds {
			total[ctrl] += v

			highest[ctrl] = append(highest[ctrl], namedCount{
				name:  name,
				count: v,
			})
		}
		sort.Slice(highest[ctrl], func(i, j int) bool {
			return highest[ctrl][i].count > highest[ctrl][j].count
		})
	}

	order := make([]string, 0, len(total))
	for k := range total {
		order = append(order, k)
	}
	sort.Strings(order)

	// Report
	for _, ctrl := range order {
		t := total[ctrl]
		fmt.Fprintf(w, "\t%s start-count: %d\n", ctrl, t)

		n := r.highestListNum
		h := highest[ctrl]
		if num := len(h); num < n {
			n = num
		}

		for i := 0; i < n; i++ {
			counter := highest[ctrl][i]
			fmt.Fprintf(w, "\t  - max: %q with: %d\n", counter.name, counter.count)
		}
		fmt.Fprintln(w, "")
	}
	fmt.Fprintln(w, "")
}
