// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package series

import (
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// Base represents an OS/Channel.
// Bases can also be converted to and from a series string.
type Base struct {
	Name string
	// Channel is track[/risk/branch].
	// eg "22.04" or "22.04/stable" etc.
	Channel Channel
}

// ParseBase constructs a Base from the os name and channel string.
func ParseBase(name string, channel string) (Base, error) {
	if name == "" || channel == "" {
		return Base{}, errors.NotValidf("missing base name or channel")
	}
	ch, err := ParseChannelNormalize(channel)
	if err != nil {
		return Base{}, errors.Annotatef(err, "parsing base %s@%s", name, channel)
	}
	if name == "centos" {
		ch.Track = ToLegacyCentosChannel(ch.Track)
	}
	return Base{Name: name, Channel: ch}, nil
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
		return Base{}, errors.Trace(err)
	}
	return Base{Name: parts[0], Channel: channel}, nil
}

// MakeDefaultBase creates a base from an os name and simple version string, eg "22.04".
func MakeDefaultBase(name string, channel string) Base {
	return Base{Name: name, Channel: MakeDefaultChannel(channel)}
}

func (b *Base) String() string {
	if b == nil || b.Name == "" {
		return ""
	}
	if b.Name == "kubernetes" {
		return b.Name
	}
	return fmt.Sprintf("%s@%s", b.Name, b.Channel)
}

func (b *Base) DisplayString() string {
	if b == nil || b.Name == "" {
		return ""
	}
	if b.Name == "kubernetes" {
		return b.Name
	}
	if b.Channel.Risk == Stable {
		return fmt.Sprintf("%s@%s", b.Name, b.Channel.Track)
	}
	return fmt.Sprintf("%s@%s", b.Name, b.Channel)
}

// GetBaseFromSeries returns the Base infor for a series.
func GetBaseFromSeries(series string) (Base, error) {
	var result Base
	osName, err := GetOSFromSeries(series)
	if err != nil {
		return result, errors.NotValidf("series %q", series)
	}
	osVersion, err := SeriesVersion(series)
	if err != nil {
		return result, errors.NotValidf("series %q", series)
	}
	result.Name = strings.ToLower(osName.String())
	result.Channel = MakeDefaultChannel(osVersion)
	return result, nil
}

func ToLegacyCentosChannel(channel string) string {
	if strings.HasPrefix(channel, "centos") {
		return channel
	}
	return "centos" + channel
}

func FromLegacyCentosChannel(series string) string {
	if strings.HasPrefix(series, "centos") {
		return strings.TrimLeft(series, "centos")
	}
	return series
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
	switch strings.ToLower(v.Name) {
	case "ubuntu":
		osSeries = ubuntuSeries
	case "centos":
		osSeries = centosSeries
		v.Channel.Track = ToLegacyCentosChannel(v.Channel.Track)
	// TODO(juju3) - remove when legacy k8s charms are gone
	case "kubernetes":
		osSeries = kubernetesSeries
	// TODO(juju3) - remove
	case "opensuse":
		osSeries = opensuseSeries
	case "windows":
		for _, vers := range windowsVersions {
			if vers.Version == v.Channel.Track {
				return v.Channel.Track, nil
			}
		}
		for _, vers := range windowsNanoVersions {
			if vers.Version == v.Channel.Track {
				return v.Channel.Track, nil
			}
		}
	}
	for s, vers := range osSeries {
		if vers.Version == v.Channel.Track {
			return string(s), nil
		}
	}
	return "", errors.NotFoundf("os %q version %q", v.Name, v.Channel.Track)
}
