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

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.api")

// The following are variables so that they can be
// changed by tests.
var (
	providerConnectDelay = 2 * time.Second
)

type apiStateCachedInfo struct {
	api.Connection
	// If cachedInfo is non-nil, it indicates that the info has been
	// newly retrieved, and should be cached in the config store.
	cachedInfo *api.Info
}

var errAborted = fmt.Errorf("aborted")

// NewAPIState creates an api.State object from an Environ
// This is almost certainly the wrong thing to do as it assumes
// the old admin password (stored as admin-secret in the config).
func NewAPIState(user names.UserTag, environ environs.Environ, dialOpts api.DialOpts) (api.Connection, error) {
	info, err := environAPIInfo(environ, user)
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
func NewAPIFromName(envName string) (api.Connection, error) {
	return newAPIClient(envName)
}

var defaultAPIOpen = api.Open

func newAPIClient(envName string) (api.Connection, error) {
	store, err := configstore.Default()
	if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := newAPIFromStore(envName, store, defaultAPIOpen)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// serverAddress returns the given string address:port as network.HostPort.
var serverAddress = func(hostPort string) (network.HostPort, error) {
	addrConnectedTo, err := network.ParseHostPorts(hostPort)
	if err != nil {
		// Should never happen, since we've just connected with it.
		return network.HostPort{}, errors.Annotatef(err, "invalid API address %q", hostPort)
	}
	return addrConnectedTo[0], nil
}

// newAPIFromStore implements the bulk of NewAPIClientFromName
// but is separate for testing purposes.
func newAPIFromStore(envName string, store configstore.Storage, apiOpen api.OpenFunc) (api.Connection, error) {
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
		logger.Debugf(
			"trying cached API connection settings - endpoints %v",
			info.APIEndpoint().Addresses,
		)
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			return apiInfoConnect(info, apiOpen, stop)
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
		return apiConfigConnect(cfg, apiOpen, stop, delay, environInfoUserTag(info))
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

	st := val0.(api.Connection)
	addrConnectedTo, err := serverAddress(st.Addr())
	if err != nil {
		return nil, err
	}
	// Even though we are about to update API addresses based on
	// APIHostPorts in cacheChangedAPIInfo, we first cache the
	// addresses based on the provider lookup. This is because older API
	// servers didn't return their HostPort information on Login, and we
	// still want to cache our connection information to them.
	if cachedInfo, ok := st.(apiStateCachedInfo); ok {
		st = cachedInfo.Connection
		if cachedInfo.cachedInfo != nil && info != nil {
			// Cache the connection settings only if we used the
			// environment config, but any errors are just logged
			// as warnings, because they're not fatal.
			err = cacheAPIInfo(st, info, cachedInfo.cachedInfo)
			if err != nil {
				logger.Warningf("cannot cache API connection settings: %v", err.Error())
			} else {
				logger.Infof("updated API connection settings cache")
			}
			addrConnectedTo, err = serverAddress(st.Addr())
			if err != nil {
				return nil, err
			}
		}
	}
	// Update API addresses if they've changed. Error is non-fatal.
	// For older servers, the environ tag or server tag may not be set.
	// if they are not, we store empty values.
	var environUUID string
	var serverUUID string
	if envTag, err := st.EnvironTag(); err == nil {
		environUUID = envTag.Id()
	}
	if serverTag, err := st.ServerTag(); err == nil {
		serverUUID = serverTag.Id()
	}
	if localerr := cacheChangedAPIInfo(info, st.APIHostPorts(), addrConnectedTo, environUUID, serverUUID); localerr != nil {
		logger.Warningf("cannot cache API addresses: %v", localerr)
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
		return 2
	}
	if _, ok := err.(*infoConnectError); ok {
		// A connection to a potentially stale cached address
		// is less important than a connection from fresh info.
		return 1
	}
	return 3
}

type infoConnectError struct {
	error
}

func environInfoUserTag(info configstore.EnvironInfo) names.UserTag {
	var username string
	if info != nil {
		username = info.APICredentials().User
	}
	if username == "" {
		username = configstore.DefaultAdminUsername
	}
	return names.NewUserTag(username)
}

// apiInfoConnect looks for endpoint on the given environment and
// tries to connect to it, sending the result on the returned channel.
func apiInfoConnect(info configstore.EnvironInfo, apiOpen api.OpenFunc, stop <-chan struct{}) (api.Connection, error) {
	endpoint := info.APIEndpoint()
	if info == nil || len(endpoint.Addresses) == 0 {
		return nil, &infoConnectError{fmt.Errorf("no cached addresses")}
	}
	logger.Infof("connecting to API addresses: %v", endpoint.Addresses)
	var environTag names.EnvironTag
	if names.IsValidEnvironment(endpoint.EnvironUUID) {
		environTag = names.NewEnvironTag(endpoint.EnvironUUID)
	}

	apiInfo := &api.Info{
		Addrs:      endpoint.Addresses,
		CACert:     endpoint.CACert,
		Tag:        environInfoUserTag(info),
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
func apiConfigConnect(cfg *config.Config, apiOpen api.OpenFunc, stop <-chan struct{}, delay time.Duration, user names.UserTag) (api.Connection, error) {
	select {
	case <-time.After(delay):
	case <-stop:
		return nil, errAborted
	}
	environ, err := environs.New(cfg)
	if err != nil {
		return nil, err
	}
	apiInfo, err := environAPIInfo(environ, user)
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

func environAPIInfo(environ environs.Environ, user names.UserTag) (*api.Info, error) {
	config := environ.Config()
	password := config.AdminSecret()
	if password == "" {
		return nil, fmt.Errorf("cannot connect to API servers without admin-secret")
	}
	info, err := environs.APIInfo(environ)
	if err != nil {
		return nil, err
	}
	info.Tag = user
	info.Password = password
	return info, nil
}

// cacheAPIInfo updates the local environment settings (.jenv file)
// with the provided apiInfo, assuming we've just successfully
// connected to the API server.
func cacheAPIInfo(st api.Connection, info configstore.EnvironInfo, apiInfo *api.Info) (err error) {
	defer errors.DeferredAnnotatef(&err, "failed to cache API credentials")
	var environUUID string
	if names.IsValidEnvironment(apiInfo.EnvironTag.Id()) {
		environUUID = apiInfo.EnvironTag.Id()
	} else {
		// For backwards-compatibility, we have to allow connections
		// with an empty UUID. Login will work for the same reasons.
		logger.Warningf("ignoring invalid cached API endpoint environment UUID %v", apiInfo.EnvironTag.Id())
	}
	hostPorts, err := network.ParseHostPorts(apiInfo.Addrs...)
	if err != nil {
		return errors.Annotatef(err, "invalid API addresses %v", apiInfo.Addrs)
	}
	addrConnectedTo, err := network.ParseHostPorts(st.Addr())
	if err != nil {
		// Should never happen, since we've just connected with it.
		return errors.Annotatef(err, "invalid API address %q", st.Addr())
	}
	addrs, hostnames, addrsChanged := PrepareEndpointsForCaching(
		info, [][]network.HostPort{hostPorts}, addrConnectedTo[0],
	)

	endpoint := configstore.APIEndpoint{
		CACert:      string(apiInfo.CACert),
		EnvironUUID: environUUID,
	}
	if addrsChanged {
		endpoint.Addresses = addrs
		endpoint.Hostnames = hostnames
	}
	info.SetAPIEndpoint(endpoint)
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

var maybePreferIPv6 = func(info configstore.EnvironInfo) bool {
	// BootstrapConfig will exist in production environments after
	// bootstrap, but for testing it's easier to mock this function.
	cfg := info.BootstrapConfig()
	result := false
	if cfg != nil {
		if val, ok := cfg["prefer-ipv6"]; ok {
			// It's optional, so if missing assume false.
			result, _ = val.(bool)
		}
	}
	return result
}

var resolveOrDropHostnames = network.ResolveOrDropHostnames

// PrepareEndpointsForCaching performs the necessary operations on the
// given API hostPorts so they are suitable for caching into the
// environment's .jenv file, taking into account the addrConnectedTo
// and the existing config store info:
//
// 1. Collapses hostPorts into a single slice.
// 2. Filters out machine-local and link-local addresses.
// 3. Removes any duplicates
// 4. Call network.SortHostPorts() on the list, respecing prefer-ipv6
// flag.
// 5. Puts the addrConnectedTo on top.
// 6. Compares the result against info.APIEndpoint.Hostnames.
// 7. If the addresses differ, call network.ResolveOrDropHostnames()
// on the list and perform all steps again from step 1.
// 8. Compare the list of resolved addresses against the cached info
// APIEndpoint.Addresses, and if changed return both addresses and
// hostnames as strings (so they can be cached on APIEndpoint) and
// set haveChanged to true.
// 9. If the hostnames haven't changed, return two empty slices and set
// haveChanged to false. No DNS resolution is performed to save time.
//
// This is used right after bootstrap to cache the initial API
// endpoints, as well as on each CLI connection to verify if the
// cached endpoints need updating.
func PrepareEndpointsForCaching(info configstore.EnvironInfo, hostPorts [][]network.HostPort, addrConnectedTo network.HostPort) (addresses, hostnames []string, haveChanged bool) {
	processHostPorts := func(allHostPorts [][]network.HostPort) []network.HostPort {
		collapsedHPs := network.CollapseHostPorts(allHostPorts)
		filteredHPs := network.FilterUnusableHostPorts(collapsedHPs)
		uniqueHPs := network.DropDuplicatedHostPorts(filteredHPs)
		// Sort the result to prefer public IPs on top (when prefer-ipv6
		// is true, IPv6 addresses of the same scope will come before IPv4
		// ones).
		preferIPv6 := maybePreferIPv6(info)
		network.SortHostPorts(uniqueHPs, preferIPv6)

		if addrConnectedTo.Value != "" {
			return network.EnsureFirstHostPort(addrConnectedTo, uniqueHPs)
		}
		// addrConnectedTo can be empty only right after bootstrap.
		return uniqueHPs
	}

	apiHosts := processHostPorts(hostPorts)
	hostsStrings := network.HostPortsToStrings(apiHosts)
	endpoint := info.APIEndpoint()
	needResolving := false

	// Verify if the unresolved addresses have changed.
	if len(apiHosts) > 0 && len(endpoint.Hostnames) > 0 {
		if addrsChanged(hostsStrings, endpoint.Hostnames) {
			logger.Debugf(
				"API hostnames changed from %v to %v - resolving hostnames",
				endpoint.Hostnames, hostsStrings,
			)
			needResolving = true
		}
	} else if len(apiHosts) > 0 {
		// No cached hostnames, most likely right after bootstrap.
		logger.Debugf("API hostnames %v - resolving hostnames", hostsStrings)
		needResolving = true
	}
	if !needResolving {
		// We're done - nothing changed.
		logger.Debugf("API hostnames unchanged - not resolving")
		return nil, nil, false
	}
	// Perform DNS resolution and check against APIEndpoints.Addresses.
	resolved := resolveOrDropHostnames(apiHosts)
	apiAddrs := processHostPorts([][]network.HostPort{resolved})
	addrsStrings := network.HostPortsToStrings(apiAddrs)
	if len(apiAddrs) > 0 && len(endpoint.Addresses) > 0 {
		if addrsChanged(addrsStrings, endpoint.Addresses) {
			logger.Infof(
				"API addresses changed from %v to %v",
				endpoint.Addresses, addrsStrings,
			)
			return addrsStrings, hostsStrings, true
		}
	} else if len(apiAddrs) > 0 {
		// No cached addresses, most likely right after bootstrap.
		logger.Infof("new API addresses to cache %v", addrsStrings)
		return addrsStrings, hostsStrings, true
	}
	// No changes.
	logger.Debugf("API addresses unchanged")
	return nil, nil, false
}

// cacheChangedAPIInfo updates the local environment settings (.jenv file)
// with the provided API server addresses if they have changed. It will also
// save the environment tag if it is available.
func cacheChangedAPIInfo(info configstore.EnvironInfo, hostPorts [][]network.HostPort, addrConnectedTo network.HostPort, environUUID, serverUUID string) error {
	addrs, hosts, addrsChanged := PrepareEndpointsForCaching(info, hostPorts, addrConnectedTo)
	logger.Debugf("cacheChangedAPIInfo: serverUUID=%q", serverUUID)
	endpoint := info.APIEndpoint()
	needCaching := false
	if endpoint.EnvironUUID != environUUID && environUUID != "" {
		endpoint.EnvironUUID = environUUID
		needCaching = true
	}
	if endpoint.ServerUUID != serverUUID && serverUUID != "" {
		endpoint.ServerUUID = serverUUID
		needCaching = true
	}
	if addrsChanged {
		endpoint.Addresses = addrs
		endpoint.Hostnames = hosts
		needCaching = true
	}
	if !needCaching {
		return nil
	}
	info.SetAPIEndpoint(endpoint)
	if err := info.Write(); err != nil {
		return err
	}
	logger.Infof("updated API connection settings cache - endpoints %v", endpoint.Addresses)
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
