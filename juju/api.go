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
	"github.com/juju/utils/parallel"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/network"
)

var logger = loggo.GetLogger("juju.juju")

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

// NewAPIConnectionParams contains the parameters for creating a new Juju API
// connection.
type NewAPIConnectionParams struct {
	// ControllerName is the name of the controller to connect to.
	ControllerName string

	// Store is the jujuclient.ClientStore from which the controller's
	// details will be fetched, and updated on address changes.
	Store jujuclient.ClientStore

	// OpenAPI is the function that will be used to open API connections.
	OpenAPI api.OpenFunc

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
	try := parallel.NewTry(0, chooseError)

	var delay time.Duration
	abortAPIConfigConnect := make(chan struct{})

	apiInfo, controller, err := connectionInfo(args)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot work out how to connect")
	}
	if len(apiInfo.Addrs) > 0 {
		try.Start(func(stop <-chan struct{}) (io.Closer, error) {
			logger.Infof("connecting to API addresses: %v", apiInfo.Addrs)
			st, err := args.OpenAPI(apiInfo, args.DialOpts)
			if err != nil {
				if isAPIError(err) {
					// It's an API error rather than
					// a connection error, so trying
					// to connect with bootstrap
					// config is not going to help.
					close(abortAPIConfigConnect)
				}
				return nil, &infoConnectError{err}
			}
			return st, nil
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
				cfg,
				apiInfo,
				controller,
				args.OpenAPI,
				stop,
				abortAPIConfigConnect,
				delay,
				args.DialOpts,
			)
			if err == nil {
				return cfg, nil
			}
			if errors.Cause(err) != errAborted {
				// Errors are swallowed by parallel.Try, so we
				// log the failure here to aid in debugging.
				logger.Debugf("failed to connect via bootstrap config: %v", err)
			}
			return cfg, errors.Annotatef(err, "connecting with bootstrap config")
		})
	} else if !errors.IsNotFound(err) || len(apiInfo.Addrs) == 0 {
		try.Close()
		return nil, err
	}

	try.Close()
	redirected := false
	val0, err := try.Result()
	if err != nil {
		if ierr, ok := err.(*infoConnectError); ok {
			// lose error encapsulation:
			err = ierr.error
		}
		redirErr, ok := errors.Cause(err).(*api.RedirectError)
		if !ok {
			return nil, errors.Trace(err)
		}
		// We've been told to connect to a different API server,
		// so do so. Note that we don't copy the account details
		// because the account on the redirected server may well
		// be different - we'll use macaroon authentication
		// directly without sending account details.
		// Copy the API info because it's possible that the
		// apiConfigConnect is still using it concurrently.
		apiInfo = &api.Info{
			ModelTag: apiInfo.ModelTag,
			Addrs:    network.HostPortsToStrings(usableHostPorts(redirErr.Servers)),
			CACert:   redirErr.CACert,
		}
		st, err := args.OpenAPI(apiInfo, args.DialOpts)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot connect to redirected address")
		}
		redirected = true
		val0 = st
	}

	st := val0.(api.Connection)
	if redirected {
		// TODO(rogpeppe) update cached model addresses.
		return st, nil
	}
	addrConnectedTo, err := serverAddress(st.Addr())
	if err != nil {
		return nil, errors.Trace(err)
	}
	// Update API addresses if they've changed. Error is non-fatal.
	// Note that in the redirection case, we won't update the addresses
	// of the controller we first connected to. This shouldn't be
	// a problem in practice because the intended scenario for
	// controllers that redirect involves them having well known
	// public addresses that won't change over time.
	hostPorts := st.APIHostPorts()
	err = updateControllerAddresses(args.Store, args.ControllerName, controller, hostPorts, addrConnectedTo)
	if err != nil {
		logger.Errorf("cannot cache API addresses: %v", err)
	}
	return st, nil
}

// connectionInfo returns connection information suitable for
// connecting to the controller and model specified in the given
// parameters. If there are no addresses known for the controller,
// it may return a *api.Info with no APIEndpoints, but all other
// information will be populated.
func connectionInfo(args NewAPIConnectionParams) (*api.Info, *jujuclient.ControllerDetails, error) {
	controller, err := args.Store.ControllerByName(args.ControllerName)
	if err != nil {
		return nil, nil, errors.Annotate(err, "cannot get controller details")
	}
	apiInfo := &api.Info{
		Addrs:  controller.APIEndpoints,
		CACert: controller.CACert,
	}
	if args.ModelUUID != "" {
		apiInfo.ModelTag = names.NewModelTag(args.ModelUUID)
	}
	if args.AccountDetails == nil {
		return apiInfo, controller, nil
	}
	account := args.AccountDetails
	if args.AccountDetails.Password != "" {
		// If a password is available, we always use
		// that.
		//
		// TODO(axw) make it invalid to store both
		// password and macaroon in accounts.yaml?
		apiInfo.Tag = names.NewUserTag(account.User)
		apiInfo.Password = account.Password
	} else if args.AccountDetails.Macaroon != "" {
		var m macaroon.Macaroon
		if err := json.Unmarshal([]byte(account.Macaroon), &m); err != nil {
			return nil, nil, errors.Trace(err)
		}
		apiInfo.Tag = names.NewUserTag(account.User)
		apiInfo.Macaroons = []macaroon.Slice{{&m}}
	} else {
		// Neither a password nor a local user macaroon was
		// found, so we'll use external macaroon authentication,
		// which requires that no tag be specified.
	}
	return apiInfo, controller, nil
}

func isAPIError(err error) bool {
	type errorCoder interface {
		ErrorCode() string
	}
	_, ok := errors.Cause(err).(errorCoder)
	return ok
}

// chooseError returns the error with the greatest importance.
func chooseError(err0, err1 error) error {
	if err0 == nil {
		return err1
	}
	if errorImportance(err0) < errorImportance(err1) {
		err0, err1 = err1, err0
	}
	logger.Debugf("discarding API open error: %v", err1)
	return err0
}

func errorImportance(err error) int {
	if err == nil {
		return 0
	}
	if errors.IsNotFound(err) {
		// An error from an actual connection attempt
		// is more interesting than the fact that there's
		// no environment info available.
		return 3
	}
	if err == errAborted {
		// A request to abort is less important than any other
		// error.
		return 1
	}
	if _, ok := err.(*infoConnectError); ok {
		// A connection to a potentially stale cached address
		// is less important than a connection from fresh info.
		return 2
	}
	return 4
}

type infoConnectError struct {
	error
}

// apiConfigConnect tries to use the info in the given environment
// configuration to connect to its API endpoint. The given
// api info is used to obtain any other parameters needed.
// info on the given environment, It only starts the attempt after the
// given delay, to allow the faster apiInfoConnect to hopefully succeed
// first.
func apiConfigConnect(
	cfg *config.Config,
	origAPIInfo *api.Info,
	controllerDetails *jujuclient.ControllerDetails,
	apiOpen api.OpenFunc,
	stop <-chan struct{},
	abort <-chan struct{},
	delay time.Duration,
	dialOpts api.DialOpts,
) (api.Connection, error) {
	select {
	case <-time.After(delay):
		// TODO(fwereade): 2016-03-17 lp:1558657
	case <-stop:
		return nil, errAborted
	case <-abort:
		return nil, errAborted
	}

	if len(controllerDetails.APIEndpoints) == 0 {
		return nil, errors.New("Juju controller is still bootstrapping, no api connection available")
	}

	environ, err := environs.New(cfg)
	if err != nil {
		return nil, errors.Annotate(err, "constructing environ")
	}
	// All api ports are the same, so just parse the first one.
	hostPort, err := network.ParseHostPort(controllerDetails.APIEndpoints[0])
	if err != nil {
		return nil, errors.Annotate(err, "parsing api port")
	}
	apiInfo, err := environs.APIInfo(controllerDetails.ControllerUUID, cfg.UUID(), controllerDetails.CACert, hostPort.Port, environ)
	if err != nil {
		return nil, errors.Annotate(err, "getting API info")
	}
	// Copy account and model details from the API information we used for the
	// initial connection attempt.
	apiInfo.Tag = origAPIInfo.Tag
	apiInfo.Password = origAPIInfo.Password
	apiInfo.Macaroons = origAPIInfo.Macaroons
	apiInfo.ModelTag = origAPIInfo.ModelTag

	st, err := apiOpen(apiInfo, dialOpts)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return apiStateCachedInfo{st, apiInfo}, nil
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
//
// TODO(rogpeppe) this function mixes too many concerns - the
// logic is difficult to follow and has non-obvious properties.
func PrepareEndpointsForCaching(
	controllerDetails jujuclient.ControllerDetails,
	hostPorts [][]network.HostPort,
	addrConnectedTo ...network.HostPort,
) (addrs, unresolvedAddrs []string, haveChanged bool) {
	processHostPorts := func(allHostPorts [][]network.HostPort) []network.HostPort {
		uniqueHPs := usableHostPorts(allHostPorts)
		network.SortHostPorts(uniqueHPs)
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

// usableHostPorts returns hps with unusable and non-unique
// host-ports filtered out.
func usableHostPorts(hps [][]network.HostPort) []network.HostPort {
	collapsed := network.CollapseHostPorts(hps)
	usable := network.FilterUnusableHostPorts(collapsed)
	unique := network.DropDuplicatedHostPorts(usable)
	return unique
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
	addrs, unresolvedAddrs, addrsChanged := PrepareEndpointsForCaching(*controllerDetails, currentHostPorts, addrConnectedTo...)
	if !addrsChanged {
		return nil
	}

	// Write the new controller data.
	controllerDetails.APIEndpoints = addrs
	controllerDetails.UnresolvedAPIEndpoints = unresolvedAddrs
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
