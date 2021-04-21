// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v9"
	"github.com/juju/errors"
)

// Source represents the source of the charm.
type Source string

// Matches attempts to match a string to a given source.
func (c Source) Matches(o string) bool {
	return string(c) == o
}

func (c Source) String() string {
	return string(c)
}

const (
	// Local represents a local charm.
	Local Source = "local"
	// CharmStore represents a charm from the now old charmstore.
	CharmStore Source = "charm-store"
	// CharmHub represents a charm from the new charmHub.
	CharmHub Source = "charm-hub"
)

// Origin holds the original source of a charm. Information about where the
// charm was installed from (charm-hub, charm-store, local) and any additional
// information we can utilise when making modelling decisions for upgrading or
// changing.
type Origin struct {
	Source Source
	Type   string
	ID     string
	Hash   string

	// Users can request a revision to be installed instead of a channel, so
	// we should model that correctly here.
	Revision *int
	Channel  *charm.Channel
	Platform Platform
}

// Platform describes the platform used to install the charm with.
type Platform struct {
	Architecture string
	OS           string
	Series       string
}

// MakePlatform creates a core charm Platform from a set of component parts.
func MakePlatform(arch, os, series string) (Platform, error) {
	if arch == "" {
		return Platform{}, errors.NotValidf("arch %q", arch)
	}
	return Platform{
		Architecture: arch,
		OS:           os,
		Series:       series,
	}, nil
}

// MustParsePlatform parses a given string or returns a panic.
func MustParsePlatform(s string) Platform {
	p, err := ParsePlatformNormalize(s)
	if err != nil {
		panic(err)
	}
	return p
}

// ParsePlatform parses a string representing a store platform.
// Serialized version of platform can be expected to conform to the following:
//
//  1. Architecture is mandatory.
//  2. OS is optional and can be dropped. Release is mandatory if OS wants
//  to be displayed.
//  3. Release is also optional.
//
// To indicate something is missing `unknown` can be used in place.
//
// Examples:
//
//  1. `<arch>/<os>/<series>`
//  2. `<arch>`
//  3. `<arch>/<series>`
//  4. `<arch>/unknown/<series>`
//
func ParsePlatform(s string) (Platform, error) {
	if s == "" {
		return Platform{}, errors.Errorf("platform cannot be empty")
	}

	p := strings.Split(s, "/")

	var arch, os, series *string
	switch len(p) {
	case 1:
		arch = &p[0]
	case 2:
		arch = &p[0]
		series = &p[1]
	case 3:
		arch, os, series = &p[0], &p[1], &p[2]
	case 4:
		arch, os, series = &p[0], &p[1], strptr(fmt.Sprintf("%s/%s", p[2], p[3]))
	default:
		return Platform{}, errors.Errorf("platform is malformed and has too many components %q", s)
	}

	platform := Platform{}
	if arch != nil {
		if *arch == "" {
			return Platform{}, errors.NotValidf("architecture in platform %q", s)
		}
		platform.Architecture = *arch
	}
	if os != nil {
		if *os == "" {
			return Platform{}, errors.NotValidf("os in platform %q", s)
		}
		platform.OS = *os
	}
	if series != nil {
		if *series == "" {
			return Platform{}, errors.NotValidf("series in platform %q", s)
		}
		platform.Series = *series
	}

	return platform, nil
}

func strptr(s string) *string {
	return &s
}

// ParsePlatformNormalize parses a string presenting a store platform.
// The returned platform's architecture, os and series are normalized.
func ParsePlatformNormalize(s string) (Platform, error) {
	platform, err := ParsePlatform(s)
	if err != nil {
		return Platform{}, errors.Trace(err)
	}
	return platform.Normalize(), nil
}

// Normalize the platform with normalized architecture, os and series.
func (p Platform) Normalize() Platform {
	os := p.OS
	if os == "unknown" {
		os = ""
	}

	series := p.Series
	if series == "unknown" {
		os = ""
		series = ""
	}

	return Platform{
		Architecture: p.Architecture,
		OS:           os,
		Series:       series,
	}
}

func (p Platform) String() string {
	path := p.Architecture
	if os := p.OS; os != "" {
		path = fmt.Sprintf("%s/%s", path, os)
	}
	if series := p.Series; series != "" {
		path = fmt.Sprintf("%s/%s", path, series)
	}

	return path
}
