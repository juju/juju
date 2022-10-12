// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/juju/errors"
)

// Base represents an OS/Channel.
// Bases can also be converted to and from a series string.
type Base struct {
	OS string
	// Channel is track[/risk/branch].
	// eg "22.04" or "22.04/stable" etc.
	Channel Channel
}

// ParseBase constructs a Base from the os and channel string.
func ParseBase(os string, channel string) (Base, error) {
	if channel == "kubernetes" {
		logger.Criticalf("%s", debug.Stack())
	}

	if os == "" && channel == "" {
		return Base{}, nil
	}
	if os == "" || channel == "" {
		return Base{}, errors.NotValidf("missing base os or channel")
	}
	ch, err := ParseChannelNormalize(channel)
	if err != nil {
		return Base{}, errors.Annotatef(err, "parsing base %s:%s", os, channel)
	}
	return Base{OS: strings.ToLower(os), Channel: ch}, nil
}

// MakeDefaultBase creates a base from an os and simple version string, eg "22.04".
func MakeDefaultBase(os string, channel string) Base {
	if channel == "kubernetes" {
		logger.Criticalf("%s", debug.Stack())
	}

	return Base{OS: os, Channel: MakeDefaultChannel(channel)}
}

func (b Base) String() string {
	if b.OS == "" {
		return ""
	}
	//if b.OS == "kubernetes" {
	//	return b.OS
	//}
	return fmt.Sprintf("%s:%s", b.OS, b.Channel)
}

func (b Base) IsCompatible(other Base) bool {
	return b.OS == other.OS && b.Channel.Track == other.Channel.Track
}

func (b *Base) DisplayString() string {
	if b == nil || b.OS == "" {
		return ""
	}
	if b.OS == "kubernetes" {
		return b.OS
	}
	if b.Channel.Risk == Stable {
		return fmt.Sprintf("%s:%s", b.OS, b.Channel.Track)
	}
	return fmt.Sprintf("%s:%s", b.OS, b.Channel)
}

// GetBaseFromSeries returns the Base infor for a series.
func GetBaseFromSeries(series string) (Base, error) {
	if series == "kubernetes" {
		logger.Criticalf("%s", debug.Stack())
	}
	var result Base
	osName, err := GetOSFromSeries(series)
	if err != nil {
		return result, errors.NotValidf("series %q", series)
	}
	osVersion, err := SeriesVersion(series)
	if err != nil {
		return result, errors.NotValidf("series %q", series)
	}
	result.OS = strings.ToLower(osName.String())
	result.Channel = MakeDefaultChannel(osVersion)
	return result, nil
}

// GetSeriesFromChannel gets the series from os name and channel.
func GetSeriesFromChannel(name string, channel string) (string, error) {
	base, err := ParseBase(name, channel)
	if err != nil {
		return "", errors.Trace(err)
	}
	return GetSeriesFromBase(base)
}

// GetSeriesFromBase returns the series name for a
// given Base. This is needed to support legacy series.
func GetSeriesFromBase(v Base) (string, error) {
	var osSeries map[SeriesName]seriesVersion
	switch strings.ToLower(v.OS) {
	case "ubuntu":
		osSeries = ubuntuSeries
	case "centos":
		osSeries = centosSeries
	}
	for s, vers := range osSeries {
		if vers.Version == v.Channel.Track {
			return string(s), nil
		}
	}
	return "", errors.NotFoundf("os %q version %q", v.OS, v.Channel.Track)
}
