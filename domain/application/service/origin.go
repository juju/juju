// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	"github.com/juju/errors"

	"github.com/juju/juju/core/arch"
	corecharm "github.com/juju/juju/core/charm"
	"github.com/juju/juju/core/os/ostype"
	"github.com/juju/juju/domain/application"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
)

func encodeCharmOrigin(origin corecharm.Origin, name string) (domaincharm.CharmOrigin, *application.Channel, application.Platform, error) {
	source, err := encodeCharmOriginSource(origin.Source)
	if err != nil {
		return domaincharm.CharmOrigin{}, nil, application.Platform{}, errors.Trace(err)
	}

	revision := -1
	if origin.Revision != nil {
		revision = *origin.Revision
	}

	channel, err := encodeChannel(origin.Channel)
	if err != nil {
		return domaincharm.CharmOrigin{}, nil, application.Platform{}, errors.Trace(err)
	}

	platform, err := encodePlatform(origin.Platform)
	if err != nil {
		return domaincharm.CharmOrigin{}, nil, application.Platform{}, errors.Trace(err)
	}

	return domaincharm.CharmOrigin{
		ReferenceName: name,
		Source:        source,
		Revision:      revision,
	}, channel, platform, nil

}

func encodeCharmOriginSource(source corecharm.Source) (domaincharm.CharmSource, error) {
	switch source {
	case corecharm.Local:
		return domaincharm.LocalSource, nil
	case corecharm.CharmHub:
		return domaincharm.CharmHubSource, nil
	default:
		return "", errors.Errorf("unknown source %q, expected local or charmhub", source)
	}
}

func encodeChannel(ch *charm.Channel) (*application.Channel, error) {
	// Empty channels (not nil), with empty strings for track, risk and branch,
	// will be normalized to "stable", so aren't officially empty.
	// We need to handle that case correctly.
	if ch == nil {
		return nil, nil
	}

	// Always ensure to normalize the channel before encoding it, so that
	// all channels saved to the database are in a consistent format.
	normalize := ch.Normalize()

	risk, err := encodeChannelRisk(normalize.Risk)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &application.Channel{
		Track:  normalize.Track,
		Risk:   risk,
		Branch: normalize.Branch,
	}, nil
}

func encodeChannelRisk(risk charm.Risk) (application.ChannelRisk, error) {
	switch risk {
	case charm.Stable:
		return application.RiskStable, nil
	case charm.Candidate:
		return application.RiskCandidate, nil
	case charm.Beta:
		return application.RiskBeta, nil
	case charm.Edge:
		return application.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q, expected stable, candidate, beta or edge", risk)
	}
}

func encodePlatform(platform corecharm.Platform) (application.Platform, error) {
	ostype, err := encodeOSType(platform.OS)
	if err != nil {
		return application.Platform{}, errors.Trace(err)
	}

	arch, err := encodeArchitecture(platform.Architecture)
	if err != nil {
		return application.Platform{}, errors.Trace(err)
	}

	return application.Platform{
		Channel:      platform.Channel,
		OSType:       ostype,
		Architecture: arch,
	}, nil
}

func encodeOSType(os string) (application.OSType, error) {
	switch ostype.OSTypeForName(os) {
	case ostype.Ubuntu:
		return domaincharm.Ubuntu, nil
	default:
		return 0, errors.Errorf("unknown os type %q, expected ubuntu", os)
	}
}

func encodeArchitecture(a string) (application.Architecture, error) {
	switch a {
	case arch.AMD64:
		return domaincharm.AMD64, nil
	case arch.ARM64:
		return domaincharm.ARM64, nil
	case arch.PPC64EL:
		return domaincharm.PPC64EL, nil
	case arch.S390X:
		return domaincharm.S390X, nil
	case arch.RISCV64:
		return domaincharm.RISV64, nil
	default:
		return 0, errors.Errorf("unknown architecture %q, expected amd64, arm64, ppc64el, s390x or riscv64", a)
	}
}
