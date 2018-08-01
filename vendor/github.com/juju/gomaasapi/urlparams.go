// Copyright 2016 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package gomaasapi

import (
	"fmt"
	"net/url"
)

// URLParams wraps url.Values to easily add values, but skipping empty ones.
type URLParams struct {
	Values url.Values
}

// NewURLParams allocates a new URLParams type.
func NewURLParams() *URLParams {
	return &URLParams{Values: make(url.Values)}
}

// MaybeAdd adds the (name, value) pair iff value is not empty.
func (p *URLParams) MaybeAdd(name, value string) {
	if value != "" {
		p.Values.Add(name, value)
	}
}

// MaybeAddInt adds the (name, value) pair iff value is not zero.
func (p *URLParams) MaybeAddInt(name string, value int) {
	if value != 0 {
		p.Values.Add(name, fmt.Sprint(value))
	}
}

// MaybeAddBool adds the (name, value) pair iff value is true.
func (p *URLParams) MaybeAddBool(name string, value bool) {
	if value {
		p.Values.Add(name, fmt.Sprint(value))
	}
}

// MaybeAddMany adds the (name, value) for each value in values iff
// value is not empty.
func (p *URLParams) MaybeAddMany(name string, values []string) {
	for _, value := range values {
		p.MaybeAdd(name, value)
	}
}
