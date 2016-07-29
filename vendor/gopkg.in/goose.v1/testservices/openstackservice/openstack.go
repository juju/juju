package openstackservice

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"gopkg.in/goose.v1/identity"
	"gopkg.in/goose.v1/testservices/identityservice"
	"gopkg.in/goose.v1/testservices/novaservice"
	"gopkg.in/goose.v1/testservices/swiftservice"
)

// Openstack provides an Openstack service double implementation.
type Openstack struct {
	Identity identityservice.IdentityService
	// Keystone v3 supports serving both V2 and V3 at the same time
	// this will intend to emulate that behavior.
	FallbackIdentity identityservice.IdentityService
	Nova             *novaservice.Nova
	Swift            *swiftservice.Swift
	// base url of openstack endpoints, might be required to
	// simmulate response contents such as the ones from
	// identity discovery.
	url string
}

func (openstack *Openstack) AddUser(user, secret, tennant string) *identityservice.UserInfo {
	uinfo := openstack.Identity.AddUser(user, secret, tennant)
	if openstack.FallbackIdentity != nil {
		_ = openstack.FallbackIdentity.AddUser(user, secret, tennant)
	}
	return uinfo
}

// New creates an instance of a full Openstack service double.
// An initial user with the specified credentials is registered with the identity service.
func New(cred *identity.Credentials, authMode identity.AuthMode) *Openstack {
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
	userInfo := openstack.AddUser(cred.User, cred.Secrets, cred.TenantName)
	if cred.TenantName == "" {
		panic("Openstack service double requires a tenant to be specified.")
	}
	openstack.Nova = novaservice.New(cred.URL, "v2", userInfo.TenantId, cred.Region, openstack.Identity, openstack.FallbackIdentity)
	// Create the swift service using only the region base so we emulate real world deployments.
	regionParts := strings.Split(cred.Region, ".")
	baseRegion := regionParts[len(regionParts)-1]
	openstack.Swift = swiftservice.New(cred.URL, "v1", userInfo.TenantId, baseRegion, openstack.Identity, openstack.FallbackIdentity)
	openstack.url = cred.URL
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
	return &openstack
}

// SetupHTTP attaches all the needed handlers to provide the HTTP API for the Openstack service..
func (openstack *Openstack) SetupHTTP(mux *http.ServeMux) {
	openstack.Identity.SetupHTTP(mux)
	// If there is a FallbackIdentity service also register its urls.
	if openstack.FallbackIdentity != nil {
		openstack.FallbackIdentity.SetupHTTP(mux)
	}
	openstack.Nova.SetupHTTP(mux)
	openstack.Swift.SetupHTTP(mux)

	// Handle root calls to be able to return auth information or fallback
	// to Nova root handler in case its not an auth info request.
	mux.Handle("/", openstack)

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
	body := []byte(fmt.Sprintf(authInformationBody, openstack.url, openstack.url))
	// workaround for https://code.google.com/p/go/issues/detail?id=4454
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(http.StatusMultipleChoices)
	w.Write(body)
}
