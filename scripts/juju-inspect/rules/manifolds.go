// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type ManifoldsRule struct {
	includeNested bool
	counts        map[string]map[string]int
}

func NewManifoldsRule(includeNested bool) *ManifoldsRule {
	return &ManifoldsRule{
		includeNested: includeNested,
		counts:        make(map[string]map[string]int),
	}
}

func (r *ManifoldsRule) Run(name string, report Report) error {
	if _, ok := r.counts[name]; !ok {
		r.counts[name] = make(map[string]int)
	}
	var suffix string
	if strings.Contains(name, ":") {
		index := strings.Index(name, ":")
		suffix = name[index:]
	}
	r.counts[name][suffix] += len(report.Manifolds)

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

func (r *ManifoldsRule) Write(w io.Writer) {
	fmt.Fprintln(w, "Manifolds:")
	fmt.Fprintln(w, "")

	counts := make([]string, 0, len(r.counts))
	for k := range r.counts {
		counts = append(counts, k)
	}
	sort.Strings(counts)

	for _, ctrl := range counts {
		agents := r.counts[ctrl]
		for nested, t := range agents {
			fmt.Fprintf(w, "\t%s:%s has %d manifolds\n", ctrl, nested, t)
		}
	}
	fmt.Fprintln(w, "")
}

type NestedReport struct {
	Workers map[string]Worker `yaml:"workers"`
}
