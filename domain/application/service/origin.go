package service

import (
	"github.com/juju/errors"

	corecharm "github.com/juju/juju/core/charm"
	domaincharm "github.com/juju/juju/domain/application/charm"
	"github.com/juju/juju/internal/charm"
)

func encodeCharmOrigin(origin corecharm.Origin) (domaincharm.CharmOrigin, *domaincharm.Channel, error) {
	source, err := encodeCharmOriginSource(origin.Source)
	if err != nil {
		return domaincharm.CharmOrigin{}, nil, errors.Trace(err)
	}

	channel, err := encodeCharmOriginChannel(origin.Channel)
	if err != nil {
		return domaincharm.CharmOrigin{}, nil, errors.Trace(err)
	}

	revision := -1
	if origin.Revision != nil {
		revision = *origin.Revision
	}

	return domaincharm.CharmOrigin{
		Source:   source,
		Revision: revision,
	}, channel, nil

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

func encodeCharmOriginChannel(ch *charm.Channel) (*domaincharm.Channel, error) {
	// Empty channels (not nil), with empty strings for track, risk and branch,
	// will be normalized to "stable", so aren't officially empty.
	// We need to handle that case correctly.
	if ch == nil {
		return nil, nil
	}

	// Always ensure to normalize the channel before encoding it, so that
	// all channels saved to the database are in a consistent format.
	normalize := ch.Normalize()

	risk, err := encodeCharmChannelRisk(normalize.Risk)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &domaincharm.Channel{
		Track:  normalize.Track,
		Risk:   risk,
		Branch: normalize.Branch,
	}, nil
}

func encodeCharmChannelRisk(risk charm.Risk) (domaincharm.ChannelRisk, error) {
	switch risk {
	case charm.Stable:
		return domaincharm.RiskStable, nil
	case charm.Candidate:
		return domaincharm.RiskCandidate, nil
	case charm.Beta:
		return domaincharm.RiskBeta, nil
	case charm.Edge:
		return domaincharm.RiskEdge, nil
	default:
		return "", errors.Errorf("unknown risk %q, expected stable, candidate, beta or edge", risk)
	}
}
