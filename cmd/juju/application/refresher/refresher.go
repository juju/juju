// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package refresher

import (
	"fmt"

	"github.com/juju/charm/v8"
	"github.com/juju/charmrepo/v6"
	jujuclock "github.com/juju/clock"
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"gopkg.in/macaroon.v2"

	"github.com/juju/juju/cmd/juju/application/store"
	corecharm "github.com/juju/juju/core/charm"
)

// RefresherDependencies are required for any deployer to be run.
type RefresherDependencies struct {
	CharmAdder store.CharmAdder
}

// RefresherConfig is the data required to choose an refresher and then run the
// PrepareAndUpgrade.
type RefresherConfig struct {
	ApplicationName string
	CharmURL        *charm.URL
	CharmRef        string
	DeployedSeries  string
	Force           bool
	ForceSeries     bool
}

type factory struct {
	charmAdder store.CharmAdder
	clock      jujuclock.Clock
}

// NewRefresherFactory returns a factory setup with the API and
// function dependencies required by every refresher.
func NewRefresherFactory(deps RefresherDependencies) RefresherFactory {
	d := &factory{
		charmAdder: deps.CharmAdder,
		clock:      jujuclock.WallClock,
	}
	return d
}

// GetRefresher returns the correct deployer to use based on the cfg provided.
// A ModelConfigGetter and CharmStoreAdaptor needed to find the deployer.
func (d *factory) GetRefresher(cfg RefresherConfig) (Refresher, error) {
	refreshers := []func(RefresherConfig) (Refresher, error){
		d.maybeReadLocal(d.charmAdder),
	}
	for _, d := range refreshers {
		if refresh, err := d(cfg); err != nil {
			return nil, errors.Trace(err)
		} else if refresh != nil {
			return refresh, nil
		}
	}
	return nil, errors.NotFoundf("suitable Refresher")
}

func (d *factory) maybeReadLocal(charmAdder store.CharmAdder) func(RefresherConfig) (Refresher, error) {
	return func(cfg RefresherConfig) (Refresher, error) {
		return &localCharmRefresher{
			charmAdder:     charmAdder,
			charmURL:       cfg.CharmURL,
			charmRef:       cfg.CharmRef,
			deployedSeries: cfg.DeployedSeries,
			force:          cfg.Force,
			forceSeries:    cfg.ForceSeries,
		}, nil
	}
}

type localCharmRefresher struct {
	charmAdder     store.CharmAdder
	charmURL       *charm.URL
	charmRef       string
	deployedSeries string
	force          bool
	forceSeries    bool
}

func (d *localCharmRefresher) PrepareAndRefresh(ctx *cmd.Context) (*charm.URL, corecharm.Origin, *macaroon.Macaroon, error) {
	var emptyOrigin corecharm.Origin

	ch, newURL, err := charmrepo.NewCharmAtPathForceSeries(d.charmRef, d.deployedSeries, d.forceSeries)
	if err == nil {
		newName := ch.Meta().Name
		if newName != d.charmURL.Name {
			return nil, emptyOrigin, nil, errors.Errorf("cannot refresh %q to %q", d.charmURL.Name, newName)
		}
		addedURL, err := d.charmAdder.AddLocalCharm(newURL, ch, d.force)
		return addedURL, emptyOrigin, nil, errors.Trace(err)
	}

	return nil, emptyOrigin, nil, errors.Errorf("not a valid local charm")
}

func (d *localCharmRefresher) String() string {
	return fmt.Sprintf("refresh local charm")
}
