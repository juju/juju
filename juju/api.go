// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/loggo"

	"launchpad.net/juju-core/environs"
	"launchpad.net/juju-core/environs/config"
	"launchpad.net/juju-core/environs/configstore"
	"launchpad.net/juju-core/errors"
	"launchpad.net/juju-core/juju/osenv"
	"launchpad.net/juju-core/names"
	"launchpad.net/juju-core/state/api"
	"launchpad.net/juju-core/state/api/keymanager"
	"launchpad.net/juju-core/utils/parallel"
)

var logger = loggo.GetLogger("juju")

// The following are variables so that they can be
// changed by tests.
var (
	apiOpen              = api.Open
	apiClose             = (*api.State).Close
	providerConnectDelay = 2 * time.Second
)

// apiState wraps an api.State, redefining its Close method
// so we can abuse it for testing purposes.
type apiState struct {
	st *api.State
	// If cachedInfo is non-nil, it indicates that the info has been
	// newly retrieved, and should be cached in the config store.
	cachedInfo *api.Info
}

func (st apiState) Close() error {
	return apiClose(st.st)
}

// APIConn holds a connection to a juju environment and its
// associated state through its API interface.
type APIConn struct {
	Environ environs.Environ
	State   *api.State
}

var errAborted = fmt.Errorf("aborted")

// NewAPIConn returns a new Conn that uses the
// given environment. The environment must have already
// been bootstrapped.
func NewAPIConn(environ environs.Environ, dialOpts api.DialOpts) (*APIConn, error) {
	info, err := environAPIInfo(environ)
	if err != nil {
		return nil, err
	}

	st, err := apiOpen(info, dialOpts)
	// TODO(rog): handle errUnauthorized when the API handles passwords.
	if err != nil {
		return nil, err
	}
	return &APIConn{
		Environ: environ,
		State:   st,
	}, nil
}

// Close terminates the connection to the environment and releases
// any associated resources.
func (c *APIConn) Close() error {
	return apiClose(c.State)
}

// NewAPIClientFromName returns an api.Client connected to the API Server for
// the named environment. If envName is "", the default environment
// will be used.
func NewAPIClientFromName(envName string) (*api.Client, error) {
	st, err := newAPIClient(envName)
	if err != nil {
		return nil, err
	}
	return st.Client(), nil
}

// NewKeyManagerClient returns an api.keymanager.Client connected to the API Server for
// the named environment. If envName is "", the default environment will be used.
func NewKeyManagerClient(envName string) (*keymanager.Client, error) {
	st, err := newAPIClient(envName)
	if err != nil {
		return nil, err
	}
	return keymanager.NewClient(st), nil
}

func newAPIClient(envName string) (*api.State, error) {
	store, err := configstore.NewDisk(osenv.JujuHome())
	if err != nil {
		return nil, err
	}
	return newAPIFromName(envName, store)
}

// newAPIFromName implements the bulk of NewAPIClientFromName
// but is separate for testing purposes.
func newAPIFromName(envName string, store configstore.Storage) (*api.State, error) {
	// Try to read the default environment configuration file.
	// If it doesn't exist, we carry on in case
	// there's some environment info for that environment.
	// This enables people to copy environment files
	// into their .juju/environments directory and have
	// them be directly useful with no further configuration changes.
	envs, err := environs.ReadEnvirons("")
	if err == nil {
		if envName == "" {
			envName = envs.Default
		}
		if envName == "" {
			return nil, fmt.Errorf("no default environment found")
		}
	} else if !environs.IsNoEnv(err) {
		return nil, err
	}

	// Try to connect to the API concurrently using two different
	// possible sources of truth for the API endpoint. Our
	// preference is for the API endpoint cached in the API info,
	// because we know that without needing to access any remote
	// provider. However, the addresses stored there may no longer
	// be current (and the network connection may take a very long
	// time to time out) so we also try to connect using information
	// found from the provider. We only start to make that
	// connection after some suitable delay, so that in the
	// hopefully usual case, we will make the connection to the API
	// and never hit the provider. By preference we use provider
	// attributes from the config store, but for backward
	// compatibility reasons, we fall back to information from
	// ReadEnvirons if that does not exist.
	chooseError := func(err0, err1 error) error {
		if err0 == nil {
			return err1
		}
		if errorImportance(err0) < errorImportance(err1) {
			err0, err1 = err1, err0
		}
		logger.Warningf("discarding API open error: %v", err1)
		return err0
	}
	try := parallel.NewTry(0, chooseError)

	info, err := store.ReadInfo(envName)
	if err != nil && !errors.IsNotFoundError(err) {
		return nil, err
	}
	var delay time.Duration
	if info != nil && len(info.APIEndpoint().Addresses) > 0 {
		logger.Debugf("trying cached API connection settings")
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			return apiInfoConnect(store, info, stop)
		})
		// Delay the config connection until we've spent
		// some time trying to connect to the cached info.
		delay = providerConnectDelay
	} else {
		logger.Debugf("no cached API connection settings found")
	}
	try.Start(func(stop <-chan struct{}) (io.Closer, error) {
		return apiConfigConnect(info, envs, envName, stop, delay)
	})
	try.Close()
	val0, err := try.Result()
	if err != nil {
		if ierr, ok := err.(*infoConnectError); ok {
			// lose error encapsulation:
			err = ierr.error
		}
		return nil, err
	}
	val := val0.(apiState)

	if val.cachedInfo != nil && info != nil {
		// Cache the connection settings only if we used the
		// environment config, but any errors are just logged
		// as warnings, because they're not fatal.
		err = cacheAPIInfo(info, val.cachedInfo)
		if err != nil {
			logger.Warningf(err.Error())
		} else {
			logger.Debugf("updated API connection settings cache")
		}
	}
	return val.st, nil
}

func errorImportance(err error) int {
	if err == nil {
		return 0
	}
	if errors.IsNotFoundError(err) {
		// An error from an actual connection attempt
		// is more interesting than the fact that there's
		// no environment info available.
		return 1
	}
	if _, ok := err.(*infoConnectError); ok {
		// A connection to a potentially stale cached address
		// is less important than a connection from fresh info.
		return 2
	}
	return 3
}

type infoConnectError struct {
	error
}

// apiInfoConnect looks for endpoint on the given environment and
// tries to connect to it, sending the result on the returned channel.
func apiInfoConnect(store configstore.Storage, info configstore.EnvironInfo, stop <-chan struct{}) (apiState, error) {
	endpoint := info.APIEndpoint()
	if info == nil || len(endpoint.Addresses) == 0 {
		return apiState{}, &infoConnectError{fmt.Errorf("no cached addresses")}
	}
	logger.Infof("connecting to API addresses: %v", endpoint.Addresses)
	apiInfo := &api.Info{
		Addrs:    endpoint.Addresses,
		CACert:   []byte(endpoint.CACert),
		Tag:      names.UserTag(info.APICredentials().User),
		Password: info.APICredentials().Password,
	}
	st, err := apiOpen(apiInfo, api.DefaultDialOpts())
	if err != nil {
		return apiState{}, &infoConnectError{err}
	}
	return apiState{st, nil}, err
}

// apiConfigConnect looks for configuration info on the given environment,
// and tries to use an Environ constructed from that to connect to
// its endpoint. It only starts the attempt after the given delay,
// to allow the faster apiInfoConnect to hopefully succeed first.
// It returns nil if there was no configuration information found.
func apiConfigConnect(info configstore.EnvironInfo, envs *environs.Environs, envName string, stop <-chan struct{}, delay time.Duration) (apiState, error) {
	var cfg *config.Config
	var err error
	if info != nil && len(info.BootstrapConfig()) > 0 {
		cfg, err = config.New(config.NoDefaults, info.BootstrapConfig())
	} else if envs != nil {
		cfg, err = envs.Config(envName)
		if errors.IsNotFoundError(err) {
			return apiState{}, err
		}
	} else {
		return apiState{}, errors.NotFoundf("environment %q", envName)
	}
	select {
	case <-time.After(delay):
	case <-stop:
		return apiState{}, errAborted
	}
	environ, err := environs.New(cfg)
	if err != nil {
		return apiState{}, err
	}
	apiInfo, err := environAPIInfo(environ)
	if err != nil {
		return apiState{}, err
	}
	st, err := apiOpen(apiInfo, api.DefaultDialOpts())
	// TODO(rog): handle errUnauthorized when the API handles passwords.
	if err != nil {
		return apiState{}, err
	}
	return apiState{st, apiInfo}, nil
}

func environAPIInfo(environ environs.Environ) (*api.Info, error) {
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
	return info, nil
}

// cacheAPIInfo updates the local environment settings (.jenv file)
// with the provided apiInfo, assuming we've just successfully
// connected to the API server.
func cacheAPIInfo(info configstore.EnvironInfo, apiInfo *api.Info) error {
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses: apiInfo.Addrs,
		CACert:    string(apiInfo.CACert),
	})
	_, username, err := names.ParseTag(apiInfo.Tag, names.UserTagKind)
	if err != nil {
		return fmt.Errorf("not caching API connection settings: invalid API user tag: %v", err)
	}
	info.SetAPICredentials(configstore.APICredentials{
		User:     username,
		Password: apiInfo.Password,
	})
	if err := info.Write(); err != nil {
		return fmt.Errorf("cannot cache API connection settings: %v", err)
	}
	return nil
}
