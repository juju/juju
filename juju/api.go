// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names"
	"github.com/juju/utils/parallel"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
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

var defaultAPIOpen = api.Open

// NewAPIConnectionParams contains the parameters for creating a new Juju API
// connection.
type NewAPIConnectionParams struct {
	// ControllerName is the name of the controller to connect to.
	ControllerName string

	// Store is the jujuclient.ClientStore from which the controller's
	// details will be fetched, and updated on address changes.
	Store jujuclient.ClientStore

	// DialOpts contains the options used to dial the API connection.
	DialOpts api.DialOpts

	// BootstrapConfig returns the bootstrap configuration for the named
	// controller.
	BootstrapConfig func(controllerName string) (*config.Config, error)

	// AccountDetails contains the account details to use for logging
	// in to the Juju API. If this is nil, then no login will take
	// place.
	AccountDetails *jujuclient.AccountDetails

	// ModelUUID is an optional model UUID. If specified, the API connection
	// will be scoped to the model with that UUID; otherwise it will be
	// scoped to the controller.
	ModelUUID string
}

// NewAPIConnection returns an api.Connection to the specified Juju controller,
// with specified account credentials, optionally scoped to the specified model
// name.
func NewAPIConnection(args NewAPIConnectionParams) (api.Connection, error) {
	st, err := newAPIFromStore(args, defaultAPIOpen)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return st, nil
}

// newAPIFromStore implements the bulk of NewAPIConnection but is separate for
// testing purposes.
func newAPIFromStore(args NewAPIConnectionParams, apiOpen api.OpenFunc) (api.Connection, error) {

	controllerDetails, err := args.Store.ControllerByName(args.ControllerName)
	if err != nil {
		return nil, errors.Annotate(err, "getting controller details")
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
	if len(controllerDetails.APIEndpoints) > 0 {
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			return apiInfoConnect(
				controllerDetails,
				args.AccountDetails,
				args.ModelUUID, apiOpen,
				stop, args.DialOpts,
			)
		})
		// Delay the config connection until we've spent
		// some time trying to connect to the cached info.
		delay = providerConnectDelay
	} else {
		logger.Debugf("no cached API connection settings found")
	}

	// If the client has bootstrap config for the controller, we'll also
	// attempt to connect by fetching new addresses from the cloud
	// directly. This is only attempted after a delay, to give the
	// faster cached-addresses method a chance to complete.
	cfg, err := args.BootstrapConfig(args.ControllerName)
	if err == nil {
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			cfg, err := apiConfigConnect(
				cfg, args.AccountDetails,
				args.ModelUUID, apiOpen,
				stop, delay, args.DialOpts,
			)
			if err != nil {
				// Errors are swallowed by parallel.Try, so we
				// log the failure here to aid in debugging.
				logger.Debugf("failed to connect via bootstrap config: %v", err)
			}
			return cfg, err
		})
	} else if !errors.IsNotFound(err) || len(controllerDetails.APIEndpoints) == 0 {
		return nil, err
	}

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
	if localerr := updateControllerAddresses(
		args.Store, args.ControllerName, controllerDetails, hostPorts, addrConnectedTo,
	); localerr != nil {
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

// apiInfoConnect looks for endpoint on the given environment and
// tries to connect to it, sending the result on the returned channel.
func apiInfoConnect(
	controller *jujuclient.ControllerDetails,
	account *jujuclient.AccountDetails,
	modelUUID string,
	apiOpen api.OpenFunc,
	stop <-chan struct{},
	dialOpts api.DialOpts,
) (api.Connection, error) {

	logger.Infof("connecting to API addresses: %v", controller.APIEndpoints)
	apiInfo := &api.Info{
		Addrs:  controller.APIEndpoints,
		CACert: controller.CACert,
	}
	st, err := commonConnect(apiOpen, apiInfo, account, modelUUID, dialOpts)
	if err != nil {
		return nil, &infoConnectError{errors.Annotate(
			err, "connecting with cached addresses",
		)}
	}
	return st, nil
}

// apiConfigConnect looks for configuration info on the given environment,
// and tries to use an Environ constructed from that to connect to
// its endpoint. It only starts the attempt after the given delay,
// to allow the faster apiInfoConnect to hopefully succeed first.
// It returns nil if there was no configuration information found.
func apiConfigConnect(
	cfg *config.Config,
	accountDetails *jujuclient.AccountDetails,
	modelUUID string,
	apiOpen api.OpenFunc,
	stop <-chan struct{},
	delay time.Duration,
	dialOpts api.DialOpts,
) (api.Connection, error) {
	select {
	case <-time.After(delay):
		// TODO(fwereade): 2016-03-17 lp:1558657
	case <-stop:
		return nil, errAborted
	}
	environ, err := environs.New(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "constructing environ")
	}
	apiInfo, err := environs.APIInfo(environ)
	if err != nil {
		return nil, errors.Annotate(err, "getting API info")
	}
	st, err := commonConnect(apiOpen, apiInfo, accountDetails, modelUUID, dialOpts)
	if err != nil {
		return nil, errors.Annotate(err, "connecting with bootstrap config")
	}
	return apiStateCachedInfo{st, apiInfo}, nil
}

func commonConnect(
	apiOpen api.OpenFunc,
	apiInfo *api.Info,
	accountDetails *jujuclient.AccountDetails,
	modelUUID string,
	dialOpts api.DialOpts,
) (api.Connection, error) {
	if accountDetails != nil {
		// We only set the tag if either a password or
		// macaroon is found in the accounts.yaml file.
		// If neither is found, we'll use external
		// macaroon authentication which requires that
		// no tag be specified.
		userTag := names.NewUserTag(accountDetails.User)
		if accountDetails.Password != "" {
			// If a password is available, we always use
			// that.
			//
			// TODO(axw) make it invalid to store both
			// password and macaroon in accounts.yaml?
			apiInfo.Tag = userTag
			apiInfo.Password = accountDetails.Password
		} else if accountDetails.Macaroon != "" {
			var m macaroon.Macaroon
			if err := json.Unmarshal([]byte(accountDetails.Macaroon), &m); err != nil {
				return nil, errors.Trace(err)
			}
			apiInfo.Tag = userTag
			apiInfo.Macaroons = []macaroon.Slice{{&m}}
		}
	}
	if modelUUID != "" {
		apiInfo.ModelTag = names.NewModelTag(modelUUID)
	}
	st, err := apiOpen(apiInfo, dialOpts)
	return st, errors.Trace(err)
}

var resolveOrDropHostnames = network.ResolveOrDropHostnames

// PrepareEndpointsForCaching performs the necessary operations on the
// given API hostPorts so they are suitable for saving into the
// controller.yaml file, taking into account the addrConnectedTo
// and the existing config store info:
//
// 1. Collapses hostPorts into a single slice.
// 2. Filters out machine-local and link-local addresses.
// 3. Removes any duplicates
// 4. Call network.SortHostPorts() on the list.
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
// This is used right after bootstrap to saved the initial API
// endpoints, as well as on each CLI connection to verify if the
// saved endpoints need updating.
func PrepareEndpointsForCaching(
	controllerDetails jujuclient.ControllerDetails,
	hostPorts [][]network.HostPort,
	addrConnectedTo ...network.HostPort,
) (addresses, hostnames []string, haveChanged bool) {
	processHostPorts := func(allHostPorts [][]network.HostPort) []network.HostPort {
		collapsedHPs := network.CollapseHostPorts(allHostPorts)
		filteredHPs := network.FilterUnusableHostPorts(collapsedHPs)
		uniqueHPs := network.DropDuplicatedHostPorts(filteredHPs)
		network.SortHostPorts(uniqueHPs, false)

		for _, addr := range addrConnectedTo {
			uniqueHPs = network.EnsureFirstHostPort(addr, uniqueHPs)
		}
		return uniqueHPs
	}

	apiHosts := processHostPorts(hostPorts)
	hostsStrings := network.HostPortsToStrings(apiHosts)
	needResolving := false

	// Verify if the unresolved addresses have changed.
	if len(apiHosts) > 0 && len(controllerDetails.UnresolvedAPIEndpoints) > 0 {
		if addrsChanged(hostsStrings, controllerDetails.UnresolvedAPIEndpoints) {
			logger.Debugf(
				"API hostnames changed from %v to %v - resolving hostnames",
				controllerDetails.UnresolvedAPIEndpoints, hostsStrings,
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
	if len(apiAddrs) > 0 && len(controllerDetails.APIEndpoints) > 0 {
		if addrsChanged(addrsStrings, controllerDetails.APIEndpoints) {
			logger.Infof(
				"API addresses changed from %v to %v",
				controllerDetails.APIEndpoints, addrsStrings,
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
	store jujuclient.ControllerStore, controllerName string,
	currentHostPorts [][]network.HostPort, addrConnectedTo ...network.HostPort,
) error {
	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	return updateControllerAddresses(
		store, controllerName, controllerDetails,
		currentHostPorts, addrConnectedTo...,
	)
}

func updateControllerAddresses(
	store jujuclient.ControllerStore,
	controllerName string, controllerDetails *jujuclient.ControllerDetails,
	currentHostPorts [][]network.HostPort, addrConnectedTo ...network.HostPort,
) error {
	// Get the new endpoint addresses.
	addrs, hosts, addrsChanged := PrepareEndpointsForCaching(*controllerDetails, currentHostPorts, addrConnectedTo...)
	if !addrsChanged {
		return nil
	}

	// Write the new controller data.
	controllerDetails.UnresolvedAPIEndpoints = hosts
	controllerDetails.APIEndpoints = addrs
	err := store.UpdateController(controllerName, *controllerDetails)
	return errors.Trace(err)
}

// serverAddress returns the given string address:port as network.HostPort.
//
// TODO(axw) fix the tests that pass invalid addresses, and drop this.
var serverAddress = func(hostPort string) (network.HostPort, error) {
	addrConnectedTo, err := network.ParseHostPorts(hostPort)
	if err != nil {
		// Should never happen, since we've just connected with it.
		return network.HostPort{}, errors.Annotatef(err, "invalid API address %q", hostPort)
	}
	return addrConnectedTo[0], nil
}
