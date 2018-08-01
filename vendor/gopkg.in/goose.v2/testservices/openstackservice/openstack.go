package openstackservice

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"

	"crypto/x509"
	"gopkg.in/goose.v2/identity"
	"gopkg.in/goose.v2/testservices/identityservice"
	"gopkg.in/goose.v2/testservices/neutronmodel"
	"gopkg.in/goose.v2/testservices/neutronservice"
	"gopkg.in/goose.v2/testservices/novaservice"
	"gopkg.in/goose.v2/testservices/swiftservice"
)

const (
	Identity = "identity"
	Neutron  = "neutron"
	Nova     = "nova"
	Swift    = "swift"
)

// Openstack provides an Openstack service double implementation.
type Openstack struct {
	Identity identityservice.IdentityService
	// Keystone v3 supports serving both V2 and V3 at the same time
	// this will intend to emulate that behavior.
	FallbackIdentity identityservice.IdentityService
	Nova             *novaservice.Nova
	Neutron          *neutronservice.Neutron
	Swift            *swiftservice.Swift
	muxes            map[string]*http.ServeMux
	servers          map[string]*httptest.Server
	// base url of openstack endpoints, might be required to
	// simmulate response contents such as the ones from
	// identity discovery.
	URLs map[string]string
}

func (openstack *Openstack) AddUser(user, secret, project, authDomain string) *identityservice.UserInfo {
	uinfo := openstack.Identity.AddUser(user, secret, project, authDomain)
	if openstack.FallbackIdentity != nil {
		_ = openstack.FallbackIdentity.AddUser(user, secret, project, authDomain)
	}
	return uinfo
}

// New creates an instance of a full Openstack service double.
// An initial user with the specified credentials is registered with the
// identity service. This service double manages the httpServers necessary
// for Neturon, Nova, Swift and Identity services
func New(cred *identity.Credentials, authMode identity.AuthMode, useTLS bool) (*Openstack, []string) {
	openstack, logMsgs := NewNoSwift(cred, authMode, useTLS)

	var server *httptest.Server
	if useTLS {
		server = httptest.NewTLSServer(nil)
	} else {
		server = httptest.NewServer(nil)
	}
	logMsgs = append(logMsgs, "swift service started: "+server.URL)
	openstack.muxes[Swift] = http.NewServeMux()
	server.Config.Handler = openstack.muxes[Swift]
	openstack.URLs[Swift] = server.URL
	openstack.servers[Swift] = server

	// Create the swift service using only the region base so we emulate real world deployments.
	regionParts := strings.Split(cred.Region, ".")
	baseRegion := regionParts[len(regionParts)-1]
	openstack.Swift = swiftservice.New(openstack.URLs["swift"], "v1", cred.TenantName, baseRegion, openstack.Identity, openstack.FallbackIdentity)
	// Create container and add image metadata endpoint so that product-streams URLs are included
	// in the keystone catalog.
	err := openstack.Swift.AddContainer("imagemetadata")
	if err != nil {
		panic(fmt.Errorf("setting up image metadata container: %v", err))
	}
	url := openstack.Swift.Endpoints()[0].PublicURL
	serviceDef := identityservice.V2Service{
		Name: "simplestreams",
		Type: "product-streams",
		Endpoints: []identityservice.Endpoint{
			{PublicURL: url + "/imagemetadata", Region: cred.Region},
		}}
	service3Def := identityservice.V3Service{
		Name:      "simplestreams",
		Type:      "product-streams",
		Endpoints: identityservice.NewV3Endpoints("", "", url+"/imagemetadata", cred.Region),
	}

	openstack.Identity.AddService(identityservice.Service{V2: serviceDef, V3: service3Def})
	// Add public bucket endpoint so that juju-tools URLs are included in the keystone catalog.
	serviceDef = identityservice.V2Service{
		Name: "juju",
		Type: "juju-tools",
		Endpoints: []identityservice.Endpoint{
			{PublicURL: url, Region: cred.Region},
		}}
	service3Def = identityservice.V3Service{
		Name:      "juju",
		Type:      "juju-tools",
		Endpoints: identityservice.NewV3Endpoints("", "", url, cred.Region),
	}

	openstack.Identity.AddService(identityservice.Service{V2: serviceDef, V3: service3Def})
	return openstack, logMsgs
}

// NewNoSwift creates an instance of a partial Openstack service double.
// An initial user with the specified credentials is registered with the
// identity service. This service double manages the httpServers necessary
// for Nova and Identity services
func NewNoSwift(cred *identity.Credentials, authMode identity.AuthMode, useTLS bool) (*Openstack, []string) {
	var openstack Openstack
	if authMode == identity.AuthKeyPair {
		openstack = Openstack{
			Identity: identityservice.NewKeyPair(),
		}
	} else if authMode == identity.AuthUserPassV3 {
		openstack = Openstack{
			Identity:         identityservice.NewV3UserPass(),
			FallbackIdentity: identityservice.NewUserPass(),
		}
	} else {
		openstack = Openstack{
			Identity:         identityservice.NewUserPass(),
			FallbackIdentity: identityservice.NewV3UserPass(),
		}
	}
	domain := cred.ProjectDomain
	if domain == "" {
		domain = cred.UserDomain
	}
	if domain == "" {
		domain = cred.Domain
	}
	if domain == "" {
		domain = "default"
	}
	userInfo := openstack.AddUser(cred.User, cred.Secrets, cred.TenantName, domain)
	if cred.TenantName == "" {
		panic("Openstack service double requires a project to be specified.")
	}

	if useTLS {
		openstack.servers = map[string]*httptest.Server{
			Identity: httptest.NewTLSServer(nil),
			Neutron:  httptest.NewTLSServer(nil),
			Nova:     httptest.NewTLSServer(nil),
		}
	} else {
		openstack.servers = map[string]*httptest.Server{
			Identity: httptest.NewServer(nil),
			Neutron:  httptest.NewServer(nil),
			Nova:     httptest.NewServer(nil),
		}
	}

	openstack.muxes = map[string]*http.ServeMux{
		Identity: http.NewServeMux(),
		Neutron:  http.NewServeMux(),
		Nova:     http.NewServeMux(),
	}

	for k, v := range openstack.servers {
		v.Config.Handler = openstack.muxes[k]
	}

	cred.URL = openstack.servers[Identity].URL
	openstack.URLs = make(map[string]string)
	var logMsgs []string
	for k, v := range openstack.servers {
		openstack.URLs[k] = v.URL
		logMsgs = append(logMsgs, k+" service started: "+openstack.URLs[k])
	}

	openstack.Nova = novaservice.New(openstack.URLs[Nova], "v2", userInfo.TenantId, cred.Region, openstack.Identity, openstack.FallbackIdentity)
	openstack.Neutron = neutronservice.New(openstack.URLs[Neutron], "v2.0", userInfo.TenantId, cred.Region, openstack.Identity, openstack.FallbackIdentity)

	return &openstack, logMsgs
}

// Certificate returns the x509 certificate of the specified service.
func (openstack *Openstack) Certificate(serviceName string) (*x509.Certificate, error) {
	service, ok := openstack.servers[serviceName]
	if ok {
		return service.Certificate(), nil
	}
	return nil, fmt.Errorf("%q not a running service", serviceName)
}

// UseNeutronNetworking sets up the openstack service to use neutron networking.
func (openstack *Openstack) UseNeutronNetworking() {
	// Neutron & Nova test doubles share a neutron data model for
	// FloatingIPs, Networks & SecurityGroups
	neutronModel := neutronmodel.New()
	openstack.Nova.AddNeutronModel(neutronModel)
	openstack.Neutron.AddNeutronModel(neutronModel)
}

// SetupHTTP attaches all the needed handlers to provide the HTTP API for the Openstack service..
func (openstack *Openstack) SetupHTTP(mux *http.ServeMux) {
	openstack.Identity.SetupHTTP(openstack.muxes[Identity])
	// If there is a FallbackIdentity service also register its urls.
	if openstack.FallbackIdentity != nil {
		openstack.FallbackIdentity.SetupHTTP(openstack.muxes[Identity])
	}
	openstack.Neutron.SetupHTTP(openstack.muxes[Neutron])
	openstack.Nova.SetupHTTP(openstack.muxes[Nova])
	if openstack.Swift != nil {
		openstack.Swift.SetupHTTP(openstack.muxes[Swift])
	}

	// Handle root calls to be able to return auth information.
	// Neutron and Nova services must handle api version information.
	// Swift has no list version api call to make
	openstack.muxes["identity"].Handle("/", openstack)
	openstack.Nova.SetupRootHandler(openstack.muxes[Nova])
	openstack.Neutron.SetupRootHandler(openstack.muxes[Neutron])
}

// Stop closes the Openstack service double http Servers and clears the
// related http handling
func (openstack *Openstack) Stop() {
	for _, v := range openstack.servers {
		v.Config.Handler = nil
		v.Close()
	}
	for k, _ := range openstack.muxes {
		openstack.muxes[k] = nil
	}
}

const authInformationBody = `{"versions": {"values": [{"status": "stable", ` +
	`"updated": "2015-03-30T00:00:00Z", "media-types": [{"base": "application/json", ` +
	`"type": "application/vnd.openstack.identity-v3+json"}], "id": "v3.4", "links": ` +
	`[{"href": "%s/v3/", "rel": "self"}]}, {"status": "stable", "updated": ` +
	`"2014-04-17T00:00:00Z", "media-types": [{"base": "application/json", ` +
	`"type": "application/vnd.openstack.identity-v2.0+json"}], "id": "v2.0", ` +
	`"links": [{"href": "%s/v2.0/", "rel": "self"}, {"href": ` +
	`"http://docs.openstack.org/", "type": "text/html", "rel": "describedby"}]}]}}`

func (openstack *Openstack) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		openstack.Nova.HandleRoot(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	body := []byte(fmt.Sprintf(authInformationBody, openstack.URLs[Identity], openstack.URLs[Identity]))
	// workaround for https://code.google.com/p/go/issues/detail?id=4454
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusMultipleChoices)
	w.Write(body)
}
