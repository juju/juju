// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/state/api"
)

// APIConn holds a connection to a juju environment and its
// associated state through its API interface.
type APIConn struct {
	Environ environs.Environ
	State   *api.State
}

// NewAPIConn returns a new Conn that uses the
// given environment. The environment must have already
// been bootstrapped.
func NewAPIConn(environ environs.Environ, dialOpts api.DialOpts) (*APIConn, error) {
	_, info, err := environ.StateInfo()
	if err != nil {
		return nil, err
	}
	info.Tag = "user-admin"
	password := environ.Config().AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("cannot connect without admin-secret")
	}
	info.Password = password

	st, err := api.Open(info, dialOpts)
	// TODO(rog): handle errUnauthorized when the API handles passwords.
	if err != nil {
		return nil, err
	}
	// TODO(rog): implement updateSecrets (see Conn.updateSecrets)
	return &APIConn{
		Environ: environ,
		State:   st,
	}, nil
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *APIConn) Close() error {
	return c.State.Close()
}

// NewAPIClientFromName returns an api.Client connected to the API Server for
// the named environment. If envName is "", the default environment
// will be used.
func OpenAPI(envName string) (*api.State, error) {
	store, err := configstore.NewDisk(config.JujuHomePath("environments"))
	if err != nil {
		return nil, err
	}
	stop := make(chan struct{})
	defer close(stop)
	cfgResult, defaultName := apiConfigConnect(envName, stop)
	var infoResult <-chan apiOpenResult
	if defaultName != "" {
		// There's no easy way to pick a default if there's
		// no configuration file to tell us.
		infoResult = apiInfoConnect(store, defaultName)
	}
	if infoResult == nil && cfgResult == nil {
		return nil, errors.NotFoundf("environment %q", envName)
	}
	var (
		st *api.State
		infoErr error
		cfgErr error
	)
	for st == nil && (infoResult != nil || cfgResult != nil) {
		select {
		case r := <-infoResult:
			st = r.st
			infoErr = r.err
			infoResult = nil
		case r := <-cfgResult:
			st = r.st
			cfgErr = r.err
			cfgResult = nil
		}
	}
	if st != nil {
		return st, nil
	}
	if cfgErr != nil {
		// Return the error from the configuration lookup if we
		// have one, because that should be using the most current
		// information.
		return nil, cfgErr
	}
	return nil, infoErr
}

type apiOpenResult struct {
	st *api.State
	err error
}


oops - apiInfoConnect needs info from apiConfigConnect (the default name)
and vice versa (the delay).


// apiInfoConnect looks for endpoint on the given environment and
// tries to connect to it, sending the result on the returned channel.
func apiInfoConnect(store environs.ConfigStorage, envName string) <-chan apiOpenResult {
	info, err := store.EnvironInfo(envName)
	if err != nil && !errors.IsNotFoundError(err) {
		log.Warningf("cannot load environment information for %q: %v", err)
		return nil
	}
	if info == nil || len(info.APIEndpoint.Addresses) > 0 {
		return nil
	}
	resultc := make(chan stateOpenResult, 1)
	go func() {
		st, err := api.Open(api.Info{
			Addrs: info.Endpoint.APIAddresses,
			CACert: info.Endpoint.CACert,
			Tag: "user-" + info.Creds.User,
			Password: info.Creds.Password,
		})
		resultc <- stateInfoResult{st, err}
	}()
	return resultc
}

// apiConfigConnect looks for configuration info on the given environment,
// and tries to use an Environ constructed from that to connect to
// its endpoint. It only starts the attempt after the given delay,
// to allow the faster apiInfoConnect to hopefully succeed first.
func apiConfigConnect(envName string, stop chan struct{}, delay time.Duration) (string, <-chan apiOpenResult {
	cfg, err := environs.ConfigForName(environName)
	if errors.IsNotFoundError(err) {
		return envName, nil
	}
	resultc := make(chan stateOpenResult, 1)
	connect := func() (*api.State, error) {
		if err != nil {
			return nil, err
		}
		select {
		case <-time.After(delay):
		case <-stop:
			return nil, fmt.Errorf("aborted")
		}
		environ, err := environs.New(cfg)
		if err != nil {
			return nil, err
		}
		return NewAPIConn(environ, api.DefaultDialOpts())
	}
	go func() {
		st, err := connect()
		resultc <- stateOpenResult{st, err}
	}()
	return cfg.Name(), resultc
}
