// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/charm/v10"
	"github.com/juju/errors"

	coreseries "github.com/juju/juju/core/series"
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

	// InstanceKey is an optional unique string associated with the application.
	// To assist with keeping KPI data in charmhub, it must be the same for every
	// charmhub Refresh action for the Refresh api endpoint related to an
	// application. For all other actions, a random uuid will used when the request
	// is sent. Create with the charmhub.CreateInstanceKey method. LP: 1944582
	InstanceKey string
}

// Platform describes the platform used to install the charm with.
type Platform struct {
	Architecture string
	OS           string
	Channel      string
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
//     to be displayed.
//  3. Release is also optional.
//
// To indicate something is missing `unknown` can be used in place.
//
// Examples:
//
//  1. `<arch>/<os>/<channel>`
//  2. `<arch>`
//  3. `<arch>/<series>`
//  4. `<arch>/unknown/<series>`
func ParsePlatform(s string) (Platform, error) {
	if s == "" {
		return Platform{}, errors.BadRequestf("platform cannot be empty")
	}

	p := strings.Split(s, "/")

	var arch, os, channel *string
	switch len(p) {
	case 1:
		arch = &p[0]
	case 2:
		arch = &p[0]
		channel = &p[1]
	case 3:
		arch, os, channel = &p[0], &p[1], &p[2]
	case 4:
		arch, os, channel = &p[0], &p[1], strptr(fmt.Sprintf("%s/%s", p[2], p[3]))
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
	if channel != nil {
		if *channel == "" {
			return Platform{}, errors.NotValidf("channel in platform %q", s)
		}
		if *channel != "unknown" {
			// Channel might be a series, eg "jammy" or an os version, eg "22.04".
			// We are transitioning away from series but still need to support it.
			// If an os version is specified, os is mandatory.
			series := *channel
			vers, err := coreseries.SeriesVersion(series)
			if err == nil {
				osType, _ := coreseries.GetOSFromSeries(series)
				platform.OS = strings.ToLower(osType.String())
				*channel = vers
			} else if platform.OS == "" {
				return Platform{}, errors.NotValidf("channel without os name in platform %q", s)
			}
			platform.Channel = *channel
		}
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

// Normalize the platform with normalized architecture, os and channel.
func (p Platform) Normalize() Platform {
	os := p.OS
	if os == "unknown" {
		os = ""
	}

	channel := p.Channel
	if channel == "unknown" {
		os = ""
		channel = ""
	}

	return Platform{
		Architecture: p.Architecture,
		OS:           os,
		Channel:      channel,
	}
}

func (p Platform) String() string {
	path := p.Architecture
	if os := p.OS; os != "" {
		path = fmt.Sprintf("%s/%s", path, os)
	}
	if channel := p.Channel; channel != "" {
		path = fmt.Sprintf("%s/%s", path, channel)
	}

	return path
}

func ChannelTrack(channel string) (string, error) {
	// Base channel can be found as either just the version `20.04` (focal)
	// or as `20.04/latest` (focal latest). We should future proof ourself
	// for now and just drop the risk on the floor.
	ch, err := charm.ParseChannel(channel)
	if err != nil {
		return "", errors.Trace(err)
	}
	if ch.Track == "" {
		return "", errors.NotValidf("channel track")
	}
	return ch.Track, nil
}
