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
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/environs/configstore"
	"github.com/juju/juju/jujuclient"
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
func NewAPIState(user names.Tag, environ environs.Environ, dialOpts api.DialOpts) (api.Connection, error) {
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

var defaultAPIOpen = api.Open

// NewAPIConnection returns an api.Connection to the specified Juju controller,
// optionally scoped to the specified model name.
func NewAPIConnection(
	store jujuclient.ClientStore,
	controllerName, modelName string,
	bClient *httpbakery.Client,
) (api.Connection, error) {
	legacyStore, err := configstore.Default()
	if err != nil {
		return nil, errors.Trace(err)
	}
	st, err := newAPIFromStore(controllerName, modelName, legacyStore, store, defaultAPIOpen, bClient)
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

// newAPIFromStore implements the bulk of NewAPIConnection but is separate for
// testing purposes.
func newAPIFromStore(controllerName, modelName string, legacyStore configstore.Storage, store jujuclient.ControllerStore, apiOpen api.OpenFunc, bClient *httpbakery.Client) (api.Connection, error) {

	// TODO(axw) this should be unnecessary once we've removed the legacy
	// configstore code. If no model name is specified, then we should
	// use the controller UUID in the login instead.
	if modelName == "" {
		modelName = configstore.AdminModelName(controllerName)
	}
	info, err := legacyStore.ReadInfo(configstore.EnvironInfoName(controllerName, modelName))
	if err != nil {
		return nil, errors.Trace(err)
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
	// and never hit the provider.
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

	var delay time.Duration
	if len(info.APIEndpoint().Addresses) > 0 {
		logger.Debugf(
			"trying cached API connection settings - endpoints %v",
			info.APIEndpoint().Addresses,
		)
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			return apiInfoConnect(info, apiOpen, stop, bClient)
		})
		// Delay the config connection until we've spent
		// some time trying to connect to the cached info.
		delay = providerConnectDelay
	} else {
		logger.Debugf("no cached API connection settings found")
	}
	try.Start(func(stop <-chan struct{}) (io.Closer, error) {
		cfg, err := getConfig(info)
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
	// Update API addresses if they've changed. Error is non-fatal.
	hostPorts := st.APIHostPorts()
	if localerr := UpdateControllerAddresses(store, legacyStore, controllerName, hostPorts, addrConnectedTo); localerr != nil {
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

func environInfoUserTag(info configstore.EnvironInfo) names.Tag {
	var username string
	if info != nil {
		username = info.APICredentials().User
	}
	if username == "" {
		return nil
	}
	return names.NewUserTag(username)
}

// apiInfoConnect looks for endpoint on the given environment and
// tries to connect to it, sending the result on the returned channel.
func apiInfoConnect(info configstore.EnvironInfo, apiOpen api.OpenFunc, stop <-chan struct{}, bClient *httpbakery.Client) (api.Connection, error) {
	endpoint := info.APIEndpoint()
	if info == nil || len(endpoint.Addresses) == 0 {
		return nil, &infoConnectError{fmt.Errorf("no cached addresses")}
	}
	logger.Infof("connecting to API addresses: %v", endpoint.Addresses)
	var modelTag names.ModelTag
	if names.IsValidModel(endpoint.ModelUUID) {
		modelTag = names.NewModelTag(endpoint.ModelUUID)
	}

	apiInfo := &api.Info{
		Addrs:    endpoint.Addresses,
		CACert:   endpoint.CACert,
		Tag:      environInfoUserTag(info),
		Password: info.APICredentials().Password,
		ModelTag: modelTag,
	}
	if apiInfo.Tag == nil {
		apiInfo.UseMacaroons = true
	}

	dialOpts := api.DefaultDialOpts()
	dialOpts.BakeryClient = bClient

	st, err := apiOpen(apiInfo, dialOpts)
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
func apiConfigConnect(cfg *config.Config, apiOpen api.OpenFunc, stop <-chan struct{}, delay time.Duration, user names.Tag) (api.Connection, error) {
	select {
	case <-time.After(delay):
		// TODO(fwereade): 2016-03-17 lp:1558657
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
func getConfig(info configstore.EnvironInfo) (*config.Config, error) {
	if len(info.BootstrapConfig()) == 0 {
		return nil, errors.NotFoundf("bootstrap config")
	}
	cfg, err := config.New(config.NoDefaults, info.BootstrapConfig())
	if err != nil {
		logger.Warningf("failed to parse bootstrap-config: %v", err)
	}
	return cfg, err
}

func environAPIInfo(environ environs.Environ, user names.Tag) (*api.Info, error) {
	config := environ.Config()
	password := config.AdminSecret()
	info, err := environs.APIInfo(environ)
	if err != nil {
		return nil, err
	}
	info.Tag = user
	info.Password = password
	if info.Tag == nil {
		info.UseMacaroons = true
	}
	return info, nil
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
func PrepareEndpointsForCaching(info configstore.EnvironInfo, hostPorts [][]network.HostPort, addrConnectedTo ...network.HostPort) (addresses, hostnames []string, haveChanged bool) {
	processHostPorts := func(allHostPorts [][]network.HostPort) []network.HostPort {
		collapsedHPs := network.CollapseHostPorts(allHostPorts)
		filteredHPs := network.FilterUnusableHostPorts(collapsedHPs)
		uniqueHPs := network.DropDuplicatedHostPorts(filteredHPs)
		// Sort the result to prefer public IPs on top (when prefer-ipv6
		// is true, IPv6 addresses of the same scope will come before IPv4
		// ones).
		preferIPv6 := maybePreferIPv6(info)
		network.SortHostPorts(uniqueHPs, preferIPv6)

		for _, addr := range addrConnectedTo {
			uniqueHPs = network.EnsureFirstHostPort(addr, uniqueHPs)
		}
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

// UpdateControllerAddresses writes any new api addresses to the client controller file.
// Controller may be specified by a UUID or name, and must already exist.
func UpdateControllerAddresses(
	store jujuclient.ControllerStore, legacystore configstore.Storage, controllerName string,
	currentHostPorts [][]network.HostPort, addrConnectedTo ...network.HostPort,
) error {

	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Trace(err)
	}

	// TODO(wallyworld) - stop storing legacy controller info when all code ported across to use new yaml files.
	var controllerInfo configstore.EnvironInfo
	var matchingModelInfos []configstore.EnvironInfo
	// Get all the controller names.
	systemNames, err := legacystore.ListSystems()
	if err != nil {
		return errors.Annotate(err, "failed to get legacy controller connection names")
	}
	// Get all the model names.
	infoNames, err := legacystore.List()
	if err != nil {
		return errors.Annotate(err, "failed to get legacy connection names")
	}
	infoNames = append(infoNames, systemNames...)

	// Figure out what we need to update.
	for _, name := range infoNames {
		info, err := legacystore.ReadInfo(name)
		if err != nil {
			return errors.Annotate(err, "failed to read legacy connection info")
		}
		ep := info.APIEndpoint()
		if ep.ServerUUID == controllerDetails.ControllerUUID {
			if ep.ServerUUID == ep.ModelUUID || ep.ModelUUID == "" {
				controllerInfo = info
			}
			matchingModelInfos = append(matchingModelInfos, info)
		}
	}
	if controllerInfo == nil {
		return errors.New("cannot update addresses, no controllers found")
	}

	// Get the new endpoint addresses.
	addrs, hosts, addrsChanged := PrepareEndpointsForCaching(controllerInfo, currentHostPorts, addrConnectedTo...)
	if !addrsChanged {
		return nil
	}

	// Write the legacy data.
	for _, info := range matchingModelInfos {
		endpoint := info.APIEndpoint()
		endpoint.Addresses = addrs
		endpoint.Hostnames = hosts
		endpoint.ServerUUID = controllerDetails.ControllerUUID
		info.SetAPIEndpoint(endpoint)
		err = info.Write()
		if err != nil {
			return errors.Annotate(err, "failed to write API endpoint to connection info")
		}
	}

	// Write the new controller data.
	controllerDetails.Servers = hosts
	controllerDetails.APIEndpoints = addrs
	err = store.UpdateController(controllerName, *controllerDetails)
	return errors.Trace(err)
}
