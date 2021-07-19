// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"fmt"
	"io"
)

type ManifoldsRule struct {
	counts map[string]int
}

func NewManifoldsRule() *ManifoldsRule {
	return &ManifoldsRule{
		counts: make(map[string]int),
	}
}

func (r *ManifoldsRule) Run(name string, report Report) error {
	r.counts[name] = len(report.Manifolds)
	return nil
}

func (r *ManifoldsRule) Write(w io.Writer) {
	fmt.Fprintln(w, "Manifolds:")
	fmt.Fprintln(w, "")
	for ctrl, t := range r.counts {
		fmt.Fprintf(w, "\t%s has %d manifolds\n", ctrl, t)
	}
	fmt.Fprintln(w, "")
}
