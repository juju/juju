// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"reflect"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/api"
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

	// AccountDetails contains the account details to use for logging
	// in to the Juju API. If this is nil, then no login will take
	// place. If AccountDetails.Password and AccountDetails.Macaroon
	// are zero, the login will be as an external user.
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
	apiInfo, controller, err := connectionInfo(args)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot work out how to connect")
	}
	if len(apiInfo.Addrs) == 0 {
		return nil, errors.New("no API addresses")
	}
	logger.Infof("connecting to API addresses: %v", apiInfo.Addrs)
	st, err := args.OpenAPI(apiInfo, args.DialOpts)
	if err != nil {
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
		st, err = args.OpenAPI(apiInfo, args.DialOpts)
		if err != nil {
			return nil, errors.Annotatef(err, "cannot connect to redirected address")
		}
		// TODO(rog) update cached model addresses.
		// TODO(rog) should we do something with the logged-in username?
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
	agentVersion := ""
	if v, ok := st.ServerVersion(); ok {
		agentVersion = v.String()
	}
	params := UpdateControllerParams{
		AgentVersion:     agentVersion,
		AddrConnectedTo:  &addrConnectedTo,
		CurrentHostPorts: hostPorts,
	}
	if host := st.PublicDNSName(); host != "" {
		params.PublicDNSName = &host
	}
	err = updateControllerDetailsFromLogin(args.Store, args.ControllerName, controller, params)
	if err != nil {
		logger.Errorf("cannot cache API addresses: %v", err)
	}

	// Process the account details obtained from login.
	var accountDetails *jujuclient.AccountDetails
	user, ok := st.AuthTag().(names.UserTag)
	if !apiInfo.SkipLogin {
		if ok {
			if accountDetails, err = args.Store.AccountDetails(args.ControllerName); err != nil {
				if !errors.IsNotFound(err) {
					logger.Errorf("cannot load local account information: %v", err)
				}
			} else {
				accountDetails.LastKnownAccess = st.ControllerAccess()
			}
		}
		if ok && !user.IsLocal() && apiInfo.Tag == nil {
			// We used macaroon auth to login; save the username
			// that we've logged in as.
			accountDetails = &jujuclient.AccountDetails{
				User:            user.Id(),
				LastKnownAccess: st.ControllerAccess(),
			}
		} else if apiInfo.Tag == nil {
			logger.Errorf("unexpected logged-in username %v", st.AuthTag())
		}
	}
	if accountDetails != nil {
		if err := args.Store.UpdateAccount(args.ControllerName, *accountDetails); err != nil {
			logger.Errorf("cannot update account information: %v", err)
		}
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
	if controller.PublicDNSName != "" {
		apiInfo.SNIHostName = controller.PublicDNSName
	}
	if args.AccountDetails == nil {
		apiInfo.SkipLogin = true
		return apiInfo, controller, nil
	}
	account := args.AccountDetails
	if account.User != "" {
		userTag := names.NewUserTag(account.User)
		if userTag.IsLocal() {
			apiInfo.Tag = userTag
		}
	}
	if args.AccountDetails.Password != "" {
		// If a password is available, we always use that.
		// If no password is recorded, we'll attempt to
		// authenticate using macaroons.
		apiInfo.Password = account.Password
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

var resolveOrDropHostnames = network.ResolveOrDropHostnames

// filterAndResolveControllerHostPorts sorts and resolves the given controller
// addresses and returns the filtered addresses and the resolved addresses
// as string slices.
//
// This is used right after bootstrap to saved the initial API
// endpoints, as well as on each CLI connection to verify if the
// saved endpoints need updating.
//
// The given controller details are used to decide whether the unresolved
// host names have changed and hence whether we can save time
// by avoiding DNS resolution.
//
// If addrConnectedTo is non-nil, the resulting slices will
// have that address at the start.
func filterAndResolveControllerHostPorts(
	controllerHostPorts [][]network.HostPort,
	controllerDetails *jujuclient.ControllerDetails,
	addrConnectedTo *network.HostPort,
) (resolvedAddrs, unresolvedAddrs []string) {
	unresolved := usableHostPorts(controllerHostPorts)
	network.SortHostPorts(unresolved)
	if addrConnectedTo != nil {
		// We know what address we connected to recently so
		// make sure it's always present at the start of the slice,
		// even if it isn't in controllerHostPorts.
		unresolved = network.EnsureFirstHostPort(*addrConnectedTo, unresolved)
	}
	unresolvedAddrs = network.HostPortsToStrings(unresolved)
	if len(unresolvedAddrs) == 0 || !addrsChanged(unresolvedAddrs, controllerDetails.UnresolvedAPIEndpoints) {
		// We have no valid addresses or the unresolved addresses haven't changed, so
		// leave things as they are.
		logger.Debugf("API hostnames unchanged - not resolving")
		return controllerDetails.APIEndpoints, controllerDetails.UnresolvedAPIEndpoints
	}
	logger.Debugf("API hostnames %v - resolving hostnames", unresolvedAddrs)

	// Perform DNS resolution and check against APIEndpoints.Addresses.
	// Note that we don't drop "unusable" addresses at this step because
	// we trust the DNS resolver to return usable addresses and it doesn't matter
	// too much if they're not.
	resolved := network.UniqueHostPorts(resolveOrDropHostnames(unresolved))
	if addrConnectedTo != nil && len(resolved) > 1 {
		// Leave the resolved connected-to address at the start.
		network.SortHostPorts(resolved[1:])
	} else {
		network.SortHostPorts(resolved)
	}
	resolvedAddrs = network.HostPortsToStrings(resolved)
	return resolvedAddrs, unresolvedAddrs
}

// usableHostPorts returns hps with unusable and non-unique
// host-ports filtered out.
func usableHostPorts(hps [][]network.HostPort) []network.HostPort {
	collapsed := network.CollapseHostPorts(hps)
	usable := network.FilterUnusableHostPorts(collapsed)
	unique := network.UniqueHostPorts(usable)
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

// UpdateControllerParams holds values used to update a controller details
// after bootstrap or a login operation.
type UpdateControllerParams struct {
	// AgentVersion is the version of the controller agent.
	AgentVersion string

	// CurrentHostPorts are the available api addresses.
	CurrentHostPorts [][]network.HostPort

	// AddrConnectedTo (when set) is an API address that has been recently
	// connected to.
	AddrConnectedTo *network.HostPort

	// PublicDNSName (when set) holds the public host name of the controller.
	PublicDNSName *string

	// ModelCount (when set) is the number of models visible to the user.
	ModelCount *int

	// ControllerMachineCount (when set) is the total number of controller machines in the environment.
	ControllerMachineCount *int

	// MachineCount (when set) is the total number of machines in the models.
	MachineCount *int
}

// UpdateControllerDetailsFromLogin writes any new api addresses and other relevant details
// to the client controller file.
// Controller may be specified by a UUID or name, and must already exist.
func UpdateControllerDetailsFromLogin(
	store jujuclient.ControllerStore, controllerName string,
	params UpdateControllerParams,
) error {
	controllerDetails, err := store.ControllerByName(controllerName)
	if err != nil {
		return errors.Trace(err)
	}
	return updateControllerDetailsFromLogin(store, controllerName, controllerDetails, params)
}

func updateControllerDetailsFromLogin(
	store jujuclient.ControllerStore,
	controllerName string, details *jujuclient.ControllerDetails,
	params UpdateControllerParams,
) error {
	// Get the new endpoint addresses.
	resolvedAddrs, unresolvedAddrs := filterAndResolveControllerHostPorts(params.CurrentHostPorts, details, params.AddrConnectedTo)

	newDetails := new(jujuclient.ControllerDetails)
	*newDetails = *details

	newDetails.AgentVersion = params.AgentVersion
	newDetails.APIEndpoints = resolvedAddrs
	newDetails.UnresolvedAPIEndpoints = unresolvedAddrs
	if params.ModelCount != nil {
		newDetails.ModelCount = params.ModelCount
	}
	if params.MachineCount != nil {
		newDetails.MachineCount = params.MachineCount
	}
	if params.ControllerMachineCount != nil {
		newDetails.ControllerMachineCount = *params.ControllerMachineCount
	}
	if params.PublicDNSName != nil {
		newDetails.PublicDNSName = *params.PublicDNSName
	}
	if reflect.DeepEqual(newDetails, details) {
		// Nothing has changed - no need to update the controller details.
		return nil
	}
	if addrsChanged(newDetails.APIEndpoints, details.APIEndpoints) {
		logger.Infof("resolved API endpoints changed from %v to %v", details.APIEndpoints, newDetails.APIEndpoints)
	}
	if addrsChanged(newDetails.UnresolvedAPIEndpoints, details.UnresolvedAPIEndpoints) {
		logger.Infof("unresolved API endpoints changed from %v to %v", details.UnresolvedAPIEndpoints, newDetails.UnresolvedAPIEndpoints)
	}
	err := store.UpdateController(controllerName, *newDetails)
	return errors.Trace(err)
}

// serverAddress returns the given string address:port as network.HostPort.
//
// TODO(axw) fix the tests that pass invalid addresses, and drop this.
var serverAddress = func(hostPort string) (network.HostPort, error) {
	hp, err := network.ParseHostPort(hostPort)
	if err != nil {
		// Should never happen, since we've just connected with it.
		return network.HostPort{}, errors.Annotatef(err, "invalid API address %q", hostPort)
	}
	return *hp, nil
}
