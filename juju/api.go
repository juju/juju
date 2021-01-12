// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package juju

import (
	"fmt"
	"net"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v4"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/jujuclient"
	"github.com/juju/juju/proxy"
)

var logger = loggo.GetLogger("juju.juju")

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

var errNoAddresses = errors.New("no API addresses")

// IsNoAddressesError reports whether the error (from NewAPIConnection) is an
// error due to the controller having no API addresses yet (likely because a
// bootstrap is still in progress).
func IsNoAddressesError(err error) bool {
	return errors.Cause(err) == errNoAddresses
}

// NewAPIConnection returns an api.Connection to the specified Juju controller,
// with specified account credentials, optionally scoped to the specified model
// name.
func NewAPIConnection(args NewAPIConnectionParams) (_ api.Connection, err error) {
	if args.OpenAPI == nil {
		args.OpenAPI = api.Open
	}
	apiInfo, controller, err := connectionInfo(args)
	if err != nil {
		return nil, errors.Annotatef(err, "cannot work out how to connect")
	}
	if len(apiInfo.Addrs) == 0 {
		return nil, errNoAddresses
	}

	if controller.Proxy != nil {
		if err := controller.Proxy.Proxier.Start(); err != nil {
			panic(err)
		}
		apiInfo.Addrs = []string{
			fmt.Sprintf("localhost:%s", controller.Proxy.Proxier.Port()),
		}
	}

	// Copy the cache so we'll know whether it's changed so that
	// we'll update the entry correctly.
	dnsCache := dnsCacheMap(controller.DNSCache).copy()
	args.DialOpts.DNSCache = dnsCache
	logger.Infof("connecting to API addresses: %v", apiInfo.Addrs)
	st, err := args.OpenAPI(apiInfo, args.DialOpts)
	if err != nil {
		redirErr, ok := errors.Cause(err).(*api.RedirectError)
		if !ok || !redirErr.FollowRedirect {
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
			Addrs:    usableHostPorts(redirErr.Servers).Strings(),
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
	defer func() {
		if err != nil {
			_ = st.Close()
		}
	}()

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
		AgentVersion:      agentVersion,
		AddrConnectedTo:   st.Addr(),
		IPAddrConnectedTo: st.IPAddr(),
		CurrentHostPorts:  hostPorts,
		DNSCache:          dnsCache,
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
	} else {
		// Optionally the account may have macaroons to use.
		apiInfo.Macaroons = account.Macaroons
	}
	return apiInfo, controller, nil
}

// usableHostPorts returns the input MachineHostPort slice as DialAddresses
// with unusable and non-unique values filtered out.
func usableHostPorts(hps []network.MachineHostPorts) network.HostPorts {
	collapsed := network.CollapseToHostPorts(hps)
	return collapsed.FilterUnusable().Unique()
}

// addrsChanged reports whether the two slices
// are different. Order is important.
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
	CurrentHostPorts []network.MachineHostPorts

	// AddrConnectedTo (when set) is an API address that has been recently
	// connected to.
	AddrConnectedTo string

	// IPAddrConnected to (when set) is the IP address of AddrConnectedTo
	// that has been recently connected to.
	IPAddrConnectedTo string

	// Proxier
	Proxier proxy.Proxier

	// DNSCache holds entries in the DNS cache.
	DNSCache map[string][]string

	// PublicDNSName (when set) holds the public host name of the controller.
	PublicDNSName *string

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
	hostPorts := usableHostPorts(params.CurrentHostPorts).Strings()
	// Move the connected-to host (if present) to the front of the address list.
	host, _, err := net.SplitHostPort(params.AddrConnectedTo)
	if err == nil {
		moveToFront(host, hostPorts)
	}
	// Move the IP address used to the front of the DNS cache entry
	// (if present) so that it will be the first address dialed.
	ipHost, _, err := net.SplitHostPort(params.IPAddrConnectedTo)
	if err == nil {
		moveToFront(ipHost, params.DNSCache[host])
	}

	newDetails := new(jujuclient.ControllerDetails)
	*newDetails = *details

	if params.Proxier != nil {
		newDetails.Proxy = &jujuclient.ProxyConfWrapper{
			Proxier: params.Proxier,
		}
	}

	newDetails.AgentVersion = params.AgentVersion
	newDetails.APIEndpoints = hostPorts
	newDetails.DNSCache = params.DNSCache
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
		logger.Infof("API endpoints changed from %v to %v", details.APIEndpoints, newDetails.APIEndpoints)
	}
	err = store.UpdateController(controllerName, *newDetails)
	return errors.Trace(err)
}

// dnsCacheMap implements api.DNSCache by
// caching entries in a map.
type dnsCacheMap map[string][]string

func (m dnsCacheMap) Lookup(host string) []string {
	return m[host]
}

func (m dnsCacheMap) copy() dnsCacheMap {
	m1 := make(dnsCacheMap)
	for host, ips := range m {
		m1[host] = append([]string{}, ips...)
	}
	return m1
}

func (m dnsCacheMap) Add(host string, ips []string) {
	m[host] = append([]string{}, ips...)
}

// moveToFront moves the given item (if present)
// to the front of the given slice.
func moveToFront(item string, xs []string) {
	for i, x := range xs {
		if x != item {
			continue
		}
		if i == 0 {
			return
		}
		copy(xs[1:], xs[0:i])
		xs[0] = item
		return
	}
}
