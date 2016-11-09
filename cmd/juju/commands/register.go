// Copyright 2015 Canonical Ltd. All rights reserved.

package commands

import (
	"net/url"

	"github.com/juju/errors"
	"github.com/juju/persistent-cookiejar"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/charms"
)

var (
	openWebBrowser = func(_ *url.URL) error { return nil }
)

type metricRegistrationPost struct {
	EnvironmentUUID string `json:"env-uuid"`
	CharmURL        string `json:"charm-url"`
	ServiceName     string `json:"service-name"`
}

var registerMeteredCharm = func(registrationURL string, state api.Connection, jar *cookiejar.Jar, charmURL string, serviceName, environmentUUID string) error {
	charmsClient := charms.NewClient(state)
	defer charmsClient.Close()
	metered, err := charmsClient.IsMetered(charmURL)
	if err != nil {
		return err
	}
	if metered {
		return errors.NotSupportedf("plans are not supported in this release of juju")
	}
	return nil
}
