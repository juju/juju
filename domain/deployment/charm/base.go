// Copyright 2020 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"fmt"
	"strings"

	"github.com/juju/juju/core/arch"
	coreerrors "github.com/juju/juju/core/errors"
	internalerrors "github.com/juju/juju/internal/errors"
)

// Base represents an OS/Channel.
// Bases can also be converted to and from a series string.
type Base struct {
	Name          string   `json:"name,omitempty"`
	Channel       Channel  `json:"channel,omitempty"`
	Architectures []string `json:"architectures,omitempty"`
}

// Validate returns with no error when the Base is valid.
func (b Base) Validate() error {
	if b.Name == "" {
		return internalerrors.Errorf("base without name not valid").Add(coreerrors.NotValid)
	}
	if b.Channel.Empty() {
		return internalerrors.Errorf("channel not valid").Add(coreerrors.NotValid)
	}
	return nil
}

// String representation of the Base.
func (b Base) String() string {
	if b.Channel.Empty() {
		panic("cannot stringify invalid base. Bases should always be validated before stringifying")
	}
	str := fmt.Sprintf("%s@%s", b.Name, b.Channel)
	if len(b.Architectures) > 0 {
		str = fmt.Sprintf("%s on %s", str, strings.Join(b.Architectures, ", "))
	}
	return str
}

// ParseBase parses a base as string in the form "os@track/risk/branch"
// and an optional list of architectures
func ParseBase(s string, archs ...string) (Base, error) {
	var err error
	base := Base{}

	segments := strings.Split(s, "@")
	if len(segments) != 2 {
		return Base{}, internalerrors.Errorf("base string must contain exactly one @. %q not valid", s).Add(coreerrors.NotValid)
	}
	base.Name = strings.ToLower(segments[0])
	channelName := segments[1]

	if channelName != "" {
		base.Channel, err = ParseChannelNormalize(channelName)
		if err != nil {
			return Base{}, internalerrors.Errorf("malformed channel in base string %q: %w", s, err)
		}
	}

	base.Architectures = make([]string, len(archs))
	for i, v := range archs {
		base.Architectures[i] = arch.NormaliseArch(v)
	}

	err = base.Validate()
	if err != nil {
		var a string
		if len(base.Architectures) > 0 {
			a = fmt.Sprintf(" with architectures %q", strings.Join(base.Architectures, ","))
		}
		return Base{}, internalerrors.Errorf("invalid base string %q%s: %w", s, a, err)
	}
	return base, nil
}
