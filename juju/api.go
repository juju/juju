// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/parallel"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/network"
	"github.com/juju/juju/state/api"
)

var logger = loggo.GetLogger("juju.api")

// The following are variables so that they can be
// changed by tests.
var (
	providerConnectDelay = 2 * time.Second
)

// apiState provides a subset of api.State's public
// interface, defined here so it can be mocked.
type apiState interface {
	Close() error
	APIHostPorts() [][]network.HostPort
	EnvironTag() string
}

type apiOpenFunc func(*api.Info, api.DialOpts) (apiState, error)

type apiStateCachedInfo struct {
	apiState
	// If cachedInfo is non-nil, it indicates that the info has been
	// newly retrieved, and should be cached in the config store.
	cachedInfo *api.Info
}

var errAborted = fmt.Errorf("aborted")

// NewAPIState creates an api.State object from an Environ
func NewAPIState(environ environs.Environ, dialOpts api.DialOpts) (*api.State, error) {
	info, err := environAPIInfo(environ)
	if err != nil {
		return nil, err
	}

	st, err := api.Open(info, dialOpts)
	if err != nil {
		return nil, err
	}
	return st, nil
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

// NewAPIFromName returns an api.State connected to the API Server for
// the named environment. If envName is "", the default environment will
// be used.
func NewAPIFromName(envName string) (*api.State, error) {
	return newAPIClient(envName)
}

func defaultAPIOpen(info *api.Info, opts api.DialOpts) (apiState, error) {
	return api.Open(info, opts)
}

func newAPIClient(envName string) (*api.State, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, err
	}
	st, err := newAPIFromStore(envName, store, defaultAPIOpen)
	if err != nil {
		return nil, err
	}
	return st.(*api.State), nil
}

// newAPIFromStore implements the bulk of NewAPIClientFromName
// but is separate for testing purposes.
func newAPIFromStore(envName string, store configstore.Storage, apiOpen apiOpenFunc) (apiState, error) {
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
	if err != nil && !errors.IsNotFound(err) {
		return nil, err
	}
	var delay time.Duration
	if info != nil && len(info.APIEndpoint().Addresses) > 0 {
		logger.Debugf("trying cached API connection settings")
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			return apiInfoConnect(store, info, apiOpen, stop)
		})
		// Delay the config connection until we've spent
		// some time trying to connect to the cached info.
		delay = providerConnectDelay
	} else {
		logger.Debugf("no cached API connection settings found")
	}
	try.Start(func(stop <-chan struct{}) (io.Closer, error) {
		cfg, err := getConfig(info, envs, envName)
		if err != nil {
			return nil, err
		}
		return apiConfigConnect(cfg, apiOpen, stop, delay)
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

	st := val0.(apiState)
	// Even though we are about to update API addresses based on
	// APIHostPorts in cacheChangedAPIInfo, we first cache the
	// addresses based on the provider lookup. This is because older API
	// servers didn't return their HostPort information on Login, and we
	// still want to cache our connection information to them.
	if cachedInfo, ok := st.(apiStateCachedInfo); ok {
		st = cachedInfo.apiState
		if cachedInfo.cachedInfo != nil && info != nil {
			// Cache the connection settings only if we used the
			// environment config, but any errors are just logged
			// as warnings, because they're not fatal.
			err = cacheAPIInfo(info, cachedInfo.cachedInfo)
			if err != nil {
				logger.Warningf("cannot cache API connection settings: %v", err.Error())
			} else {
				logger.Infof("updated API connection settings cache")
			}
		}
	}
	// Update API addresses if they've changed. Error is non-fatal.
	if localerr := cacheChangedAPIInfo(info, st.APIHostPorts(), st.EnvironTag()); localerr != nil {
		logger.Warningf("cannot failed to cache API addresses: %v", localerr)
	}
	return st, nil
}

func errorImportance(err error) int {
	if err == nil {
		return 0
	}
	if errors.IsNotFound(err) {
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
func apiInfoConnect(store configstore.Storage, info configstore.EnvironInfo, apiOpen apiOpenFunc, stop <-chan struct{}) (apiState, error) {
	endpoint := info.APIEndpoint()
	if info == nil || len(endpoint.Addresses) == 0 {
		return nil, &infoConnectError{fmt.Errorf("no cached addresses")}
	}
	logger.Infof("connecting to API addresses: %v", endpoint.Addresses)
	var environTag names.Tag
	if endpoint.EnvironUUID != "" {
		// Note: we should be validating that EnvironUUID contains a
		// valid UUID.
		environTag = names.NewEnvironTag(endpoint.EnvironUUID)
	}
	apiInfo := &api.Info{
		Addrs:      endpoint.Addresses,
		CACert:     endpoint.CACert,
		Tag:        names.NewUserTag(info.APICredentials().User),
		Password:   info.APICredentials().Password,
		EnvironTag: environTag,
	}
	st, err := apiOpen(apiInfo, api.DefaultDialOpts())
	if err != nil {
		return nil, &infoConnectError{err}
	}
	return st, nil
}

// apiConfigConnect looks for configuration info on the given environment,
// and tries to use an Environ constructed from that to connect to
// its endpoint. It only starts the attempt after the given delay,
// to allow the faster apiInfoConnect to hopefully succeed first.
// It returns nil if there was no configuration information found.
func apiConfigConnect(cfg *config.Config, apiOpen apiOpenFunc, stop <-chan struct{}, delay time.Duration) (apiState, error) {
	select {
	case <-time.After(delay):
	case <-stop:
		return nil, errAborted
	}
	environ, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	apiInfo, err := environAPIInfo(environ)
	if err != nil {
		return nil, err
	}
	st, err := apiOpen(apiInfo, api.DefaultDialOpts())
	// TODO(rog): handle errUnauthorized when the API handles passwords.
	if err != nil {
		return nil, err
	}
	return apiStateCachedInfo{st, apiInfo}, nil
}

// getConfig looks for configuration info on the given environment
func getConfig(info configstore.EnvironInfo, envs *environs.Environs, envName string) (*config.Config, error) {
	if info != nil && len(info.BootstrapConfig()) > 0 {
		cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
		if err != nil {
			logger.Warningf("failed to parse bootstrap-config: %v", err)
		}
		return cfg, err
	}
	if envs != nil {
		cfg, err := envs.Config(envName)
		if err != nil && !errors.IsNotFound(err) {
			logger.Warningf("failed to get config for environment %q: %v", envName, err)
		}
		return cfg, err
	}
	return nil, errors.NotFoundf("environment %q", envName)
}

func environAPIInfo(environ environs.Environ) (*api.Info, error) {
	config := environ.Config()
	password := config.AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("cannot connect to API servers without admin-secret")
	}
	info, err := environs.APIInfo(environ)
	if err != nil {
		return nil, err
	}
	info.Tag = names.NewUserTag("admin")
	info.Password = password
	return info, nil
}

// cacheAPIInfo updates the local environment settings (.jenv file)
// with the provided apiInfo, assuming we've just successfully
// connected to the API server.
func cacheAPIInfo(info configstore.EnvironInfo, apiInfo *api.Info) (err error) {
	defer errors.Contextf(&err, "failed to cache API credentials")
	var environUUID string
	if apiInfo.EnvironTag != nil {
		environUUID = apiInfo.EnvironTag.Id()
	}
	info.SetAPIEndpoint(configstore.APIEndpoint{
		Addresses:   apiInfo.Addrs,
		CACert:      string(apiInfo.CACert),
		EnvironUUID: environUUID,
	})
	tag, ok := apiInfo.Tag.(names.UserTag)
	if !ok {
		return errors.Errorf("apiInfo.Tag was of type %T, expecting names.UserTag", apiInfo.Tag)
	}
	info.SetAPICredentials(configstore.APICredentials{
		// This looks questionable. We have a tag, say "user-admin", but then only
		// the Id portion of the tag is recorded, "admin", so this is really a
		// username, not a tag, and cannot be reconstructed accurately.
		User:     tag.Id(),
		Password: apiInfo.Password,
	})
	return info.Write()
}

// cacheChangedAPIInfo updates the local environment settings (.jenv file)
// with the provided API server addresses if they have changed. It will also
// save the environment tag if it is available.
func cacheChangedAPIInfo(info configstore.EnvironInfo, hostPorts [][]network.HostPort, newEnvironTag string) error {
	var addrs []string
	for _, serverHostPorts := range hostPorts {
		for _, hostPort := range serverHostPorts {
			// Only cache addresses that are likely to be usable,
			// exclude localhost style ones.
			if hostPort.Scope != network.ScopeMachineLocal &&
				hostPort.Scope != network.ScopeLinkLocal {
				addrs = append(addrs, hostPort.NetAddr())
			}
		}
	}
	endpoint := info.APIEndpoint()
	changed := false
	if newEnvironTag != "" {
		tag, err := names.ParseEnvironTag(newEnvironTag)
		if err == nil {
			if environUUID := tag.Id(); endpoint.EnvironUUID != environUUID {
				changed = true
				endpoint.EnvironUUID = environUUID
			}
		} else {
			logger.Debugf("cannot parse environ tag: %v", err)
		}
	}
	if len(addrs) != 0 && addrsChanged(endpoint.Addresses, addrs) {
		logger.Debugf("API addresses changed from %q to %q", endpoint.Addresses, addrs)
		changed = true
		endpoint.Addresses = addrs
	}
	if !changed {
		return nil
	}
	info.SetAPIEndpoint(endpoint)
	if err := info.Write(); err != nil {
		return err
	}
	logger.Infof("updated API connection settings cache")
	return nil
}

// addrsChanged returns true iff the two
// slices are not equal. Order is important.
func addrsChanged(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}
