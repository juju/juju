// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package rules

import (
	"bytes"
	"fmt"
)

type ManifoldsRule struct {
	counts map[string]int
}

func NewManifoldsRule() *ManifoldsRule {
	return &ManifoldsRule{
		counts: make(map[string]int),
	}
}

func (r *ManifoldsRule) Run(name string, report Report) {
	r.counts[name] = len(report.Manifolds)
}

func (r *ManifoldsRule) Summary() string {
	return "Manifolds:"
}

func (r *ManifoldsRule) Analyse() string {
	buf := new(bytes.Buffer)
	for ctrl, t := range r.counts {
		fmt.Fprintf(buf, "%s has %d manifolds\n", ctrl, t)
	}
	return buf.String()
}
