// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

import (
	"fmt"
	"strings"

	"github.com/juju/collections/set"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/charm"
	"github.com/juju/juju/internal/errors"
)

// Base represents an OS/Channel.
// Bases can also be converted to and from a series string.
type Base struct {
	OS string
	// Channel is track[/risk/branch].
	// eg "22.04" or "22.04/stable" etc.
	Channel Channel
}

const (
	// UbuntuOS is the special value to be places in OS field of a base to
	// indicate an operating system is an Ubuntu distro
	UbuntuOS = "ubuntu"
)

// ParseBase constructs a Base from the os and channel string.
func ParseBase(os string, channel string) (Base, error) {
	if os == "" && channel == "" {
		return Base{}, nil
	}
	if os == "" || channel == "" {
		return Base{}, errors.Errorf("missing base os or channel %w", coreerrors.NotValid)
	}
	ch, err := ParseChannelNormalize(channel)
	if err != nil {
		return Base{}, errors.Errorf("parsing base %s@%s: %w", os, channel, err)
	}
	return Base{OS: strings.ToLower(os), Channel: ch}, nil
}

// ParseBaseFromString takes a string containing os and channel separated
// by @ and returns a base.
func ParseBaseFromString(b string) (Base, error) {
	parts := strings.Split(b, "@")
	if len(parts) != 2 {
		return Base{}, errors.New("expected base string to contain os and channel separated by '@'")
	}
	channel, err := ParseChannelNormalize(parts[1])
	if err != nil {
		return Base{}, errors.Capture(err)
	}
	return Base{OS: parts[0], Channel: channel}, nil
}

// ParseManifestBases transforms charm.Bases to Bases. This
// format comes out of a charm.Manifest and contains architectures
// which Base does not. Only unique non architecture Bases
// will be returned.
func ParseManifestBases(manifestBases []charm.Base) ([]Base, error) {
	if len(manifestBases) == 0 {
		return nil, errors.Errorf("base len zero").Add(coreerrors.BadRequest)
	}
	bases := make([]Base, 0)
	unique := set.NewStrings()
	for _, m := range manifestBases {
		// The data actually comes over the wire as an operating system
		// with a single architecture, not multiple ones.
		// TODO - (hml) 2023-05-18
		// There is no guarantee that every architecture has
		// the same operating systems. This logic should be
		// investigated.
		m.Architectures = []string{}
		if unique.Contains(m.String()) {
			continue
		}
		base, err := ParseBase(m.Name, m.Channel.String())
		if err != nil {
			return nil, err
		}
		bases = append(bases, base)
		unique.Add(m.String())
	}
	return bases, nil
}

// MustParseBaseFromString is like ParseBaseFromString but panics if the string
// is invalid.
func MustParseBaseFromString(b string) Base {
	base, err := ParseBaseFromString(b)
	if err != nil {
		panic(err)
	}
	return base
}

// MakeDefaultBase creates a base from an os and simple version string, eg "22.04".
func MakeDefaultBase(os string, channel string) Base {
	return Base{OS: os, Channel: MakeDefaultChannel(channel)}
}

// Empty returns true if the base is empty.
func (b Base) Empty() bool {
	return b.OS == "" && b.Channel.Empty()
}

func (b Base) String() string {
	if b.OS == "" {
		return ""
	}
	return fmt.Sprintf("%s@%s", b.OS, b.Channel)
}

// IsCompatible returns true if base other is the same underlying
// OS version, ignoring risk.
func (b Base) IsCompatible(other Base) bool {
	return b.OS == other.OS && b.Channel.Track == other.Channel.Track
}

// ubuntuLTSes lists the Ubuntu LTS releases that
// this version of Juju knows about
var ubuntuLTSes = []Base{
	MakeDefaultBase(UbuntuOS, "20.04"),
	MakeDefaultBase(UbuntuOS, "22.04"),
	MakeDefaultBase(UbuntuOS, "24.04"),
}

// IsUbuntuLTS returns true if this base is a recognised
// Ubuntu LTS.
func (b Base) IsUbuntuLTS() bool {
	for _, ubuntuLTS := range ubuntuLTSes {
		if b.IsCompatible(ubuntuLTS) {
			return true
		}
	}
	return false
}

// DisplayString returns the base string ignoring risk.
func (b Base) DisplayString() string {
	if b.Channel.Track == "" || b.OS == "" {
		return ""
	}
	return b.OS + "@" + b.Channel.DisplayString()
}
