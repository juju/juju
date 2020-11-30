// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceadapters

import (
	"github.com/juju/errors"
	"github.com/juju/names/v4"

	csclient "github.com/juju/juju/charmstore"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/charmstore"
	corestate "github.com/juju/juju/state"
)

// NewResourceOpener returns a new resource.Opener for the given unit.
//
// The caller owns the State provided. It is the caller's
// responsibility to close it.
//
// TODO(mjs): This is the entry point for a whole lot of untested shim
// code in this package. At some point this should be sorted out.
func NewResourceOpener(st *corestate.State, unitName string) (opener resource.Opener, err error) {
	unit, err := st.Unit(unitName)
	if err != nil {
		return nil, errors.Annotate(err, "loading unit")
	}

	resources, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}

	opener = &resourceOpener{
		st:     st,
		res:    resources,
		userID: unit.Tag(),
		unit:   unit,
	}
	return opener, nil
}

// resourceOpener is an implementation of server.ResourceOpener.
type resourceOpener struct {
	st     *corestate.State
	res    corestate.Resources
	userID names.Tag
	unit   *corestate.Unit
}

// OpenResource implements server.ResourceOpener.
func (ro *resourceOpener) OpenResource(name string) (o resource.Opened, err error) {
	if ro.unit == nil {
		return resource.Opened{}, errors.Errorf("missing unit")
	}
	app, err := ro.unit.Application()
	if err != nil {
		return resource.Opened{}, errors.Trace(err)
	}
	cURL, _ := ro.unit.CharmURL()

	switch cURL.Schema {
	case "cs":
		logger.Criticalf("TODO charmstore resourceOpener.OpenResource(%q)", name)
		id := csclient.CharmID{
			URL:     cURL,
			Channel: app.Channel(),
		}

		csOpener := newCharmstoreOpener(ro.st)
		client, err := csOpener.NewClient()
		if err != nil {
			return resource.Opened{}, errors.Trace(err)
		}

		cache := &charmstoreEntityCache{
			st:            ro.res,
			userID:        ro.userID,
			unit:          ro.unit,
			applicationID: ro.unit.ApplicationName(),
		}

		res, reader, err := charmstore.GetResource(charmstore.GetResourceArgs{
			Client:  client,
			Cache:   cache,
			CharmID: id,
			Name:    name,
		})
		if err != nil {
			return resource.Opened{}, errors.Trace(err)
		}

		opened := resource.Opened{
			ReadCloser: reader,
			Size: res.Size,
			Fingerprint: res.Fingerprint,
		}
		return opened, nil

	case "ch":
		logger.Criticalf("TODO charmhub resourceOpener.OpenResource(%q)", name)
		return resource.Opened{}, errors.NotValidf("TODO resourceOpener.OpenResource doesn't support charmhub yet")
		/*
		model, _ := ro.st.Model()
		modelConfig, _ := model.Config()
		charmHubURL, _ := modelConfig.CharmHubURL()
		config, _ := charmhub.CharmHubConfigFromURL(charmHubURL, logger.Child("charmhub"))
		client, _ := charmhub.NewClient(config)

		res, reader, err := client.GetResource(cURL, app.Channel(), name)
		if err != nil {
			return resource.Opened{}, errors.Trace(err)
		}

		opened := resource.Opened{
			Resource:   res,
			ReadCloser: reader,
		}
		return opened, nil
*/
	default:
		return resource.Opened{}, errors.NotValidf("invalid charm URL schema %q", cURL.Schema)
	}
}
