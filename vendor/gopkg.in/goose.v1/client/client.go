package client

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	gooseerrors "gopkg.in/goose.v1/errors"
	goosehttp "gopkg.in/goose.v1/http"
	"gopkg.in/goose.v1/identity"
	goosesync "gopkg.in/goose.v1/sync"
)

const (
	apiTokens   = "/tokens"
	apiTokensV3 = "/auth/tokens"

	// The HTTP request methods.
	GET    = "GET"
	POST   = "POST"
	PUT    = "PUT"
	DELETE = "DELETE"
	HEAD   = "HEAD"
	COPY   = "COPY"
)

// Client implementations sends service requests to an OpenStack deployment.
type Client interface {
	SendRequest(method, svcType, apiCall string, requestData *goosehttp.RequestData) (err error)
	// MakeServiceURL prepares a full URL to a service endpoint, with optional
	// URL parts. It uses the first endpoint it can find for the given service type.
	MakeServiceURL(serviceType string, parts []string) (string, error)
}

// AuthenticatingClient sends service requests to an OpenStack deployment after first validating
// a user's credentials.
type AuthenticatingClient interface {
	Client
	SetRequiredServiceTypes(requiredServiceTypes []string)
	Authenticate() error
	IsAuthenticated() bool
	Token() string
	UserId() string
	TenantId() string

	EndpointsForRegion(string) identity.ServiceURLs
	// IdentityAuthOptions returns a list of valid auth options for the given
	// openstack or error if fetching fails.
	IdentityAuthOptions() (identity.AuthOptions, error)
}

// A single http client is shared between all Goose clients.
var sharedHttpClient = goosehttp.New()

// This client sends requests without authenticating.
type client struct {
	mu         sync.Mutex
	logger     *log.Logger
	baseURL    string
	httpClient *goosehttp.Client
}

var _ Client = (*client)(nil)

// This client authenticates before sending requests.
type authenticatingClient struct {
	client

	creds    *identity.Credentials
	authMode identity.Authenticator

	auth AuthenticatingClient

	authOptions identity.AuthOptions

	// Service type to endpoint URLs for each available region
	regionServiceURLs map[string]identity.ServiceURLs

	// Service type to endpoint URLs for the authenticated region
	serviceURLs identity.ServiceURLs

	// The service types which must be available after authentication,
	// or else services which use this client will not be able to function as expected.
	requiredServiceTypes []string
	tokenId              string
	tenantId             string
	userId               string
}

func (c *authenticatingClient) EndpointsForRegion(region string) identity.ServiceURLs {
	return c.regionServiceURLs[region]
}

var _ AuthenticatingClient = (*authenticatingClient)(nil)

func NewPublicClient(baseURL string, logger *log.Logger) Client {
	client := client{baseURL: baseURL, logger: logger, httpClient: sharedHttpClient}
	return &client
}

func NewNonValidatingPublicClient(baseURL string, logger *log.Logger) Client {
	return &client{
		baseURL:    baseURL,
		logger:     logger,
		httpClient: goosehttp.NewNonSSLValidating(),
	}
}

var defaultRequiredServiceTypes = []string{"compute", "object-store"}

func newClient(creds *identity.Credentials, auth_method identity.AuthMode, httpClient *goosehttp.Client, logger *log.Logger) AuthenticatingClient {
	client_creds := *creds
	if strings.HasSuffix(client_creds.URL, "/") {
		client_creds.URL = client_creds.URL[:len(client_creds.URL)-1]
	}
	switch auth_method {
	case identity.AuthUserPassV3:
		client_creds.URL = client_creds.URL + apiTokensV3
	default:
		client_creds.URL = client_creds.URL + apiTokens
	}
	client := authenticatingClient{
		creds:                &client_creds,
		requiredServiceTypes: defaultRequiredServiceTypes,
		client:               client{logger: logger, httpClient: httpClient},
	}
	client.auth = &client
	client.authMode = identity.NewAuthenticator(auth_method, httpClient)
	return &client
}

func NewClient(creds *identity.Credentials, auth_method identity.AuthMode, logger *log.Logger) AuthenticatingClient {
	return newClient(creds, auth_method, sharedHttpClient, logger)
}

func NewNonValidatingClient(creds *identity.Credentials, auth_method identity.AuthMode, logger *log.Logger) AuthenticatingClient {
	return newClient(creds, auth_method, goosehttp.NewNonSSLValidating(), logger)
}

func (c *client) sendRequest(method, url, token string, requestData *goosehttp.RequestData) (err error) {
	if requestData.ReqValue != nil || requestData.RespValue != nil {
		err = c.httpClient.JsonRequest(method, url, token, requestData, c.logger)
	} else {
		err = c.httpClient.BinaryRequest(method, url, token, requestData, c.logger)
	}
	return
}

func (c *client) SendRequest(method, svcType, apiCall string, requestData *goosehttp.RequestData) error {
	url, _ := c.MakeServiceURL(svcType, []string{apiCall})
	return c.sendRequest(method, url, "", requestData)
}

func makeURL(base string, parts []string) string {
	if !strings.HasSuffix(base, "/") && len(parts) > 0 {
		base += "/"
	}
	return base + strings.Join(parts, "/")
}

func (c *client) MakeServiceURL(serviceType string, parts []string) (string, error) {
	return makeURL(c.baseURL, parts), nil
}

func (c *authenticatingClient) SetRequiredServiceTypes(requiredServiceTypes []string) {
	c.requiredServiceTypes = requiredServiceTypes
}

func (c *authenticatingClient) SendRequest(method, svcType, apiCall string, requestData *goosehttp.RequestData) (err error) {
	err = c.sendAuthRequest(method, svcType, apiCall, requestData)
	if gooseerrors.IsUnauthorised(err) {
		c.setToken("")
		err = c.sendAuthRequest(method, svcType, apiCall, requestData)
	}
	return
}

func (c *authenticatingClient) sendAuthRequest(method, svcType, apiCall string, requestData *goosehttp.RequestData) (err error) {
	if err = c.Authenticate(); err != nil {
		return
	}

	url, err := c.MakeServiceURL(svcType, []string{apiCall})
	if err != nil {
		return
	}
	return c.sendRequest(method, url, c.Token(), requestData)
}

func (c *authenticatingClient) MakeServiceURL(serviceType string, parts []string) (string, error) {
	if !c.IsAuthenticated() {
		return "", errors.New("cannot get endpoint URL without being authenticated")
	}
	url, ok := c.serviceURLs[serviceType]
	if !ok {
		return "", errors.New("no endpoints known for service type: " + serviceType)
	}
	return makeURL(url, parts), nil
}

// Return the relevant service endpoint URLs for this client's region.
// The region comes from the client credentials.
func (c *authenticatingClient) createServiceURLs() error {
	var serviceURLs identity.ServiceURLs = nil
	var otherServiceTypeRegions map[string][]string = make(map[string][]string)
	for region, urls := range c.regionServiceURLs {
		if regionMatches(c.creds.Region, region) {
			if serviceURLs == nil {
				serviceURLs = make(identity.ServiceURLs)
			}
			for serviceType, endpointURL := range urls {
				serviceURLs[serviceType] = endpointURL
			}
		} else {
			for serviceType := range urls {
				regions := otherServiceTypeRegions[serviceType]
				if regions == nil {
					regions = []string{}
				}
				otherServiceTypeRegions[serviceType] = append(regions, region)
			}
		}
	}
	var errorPrefix string
	var possibleRegions, missingServiceTypes []string
	if serviceURLs == nil {
		var knownRegions []string
		for r := range c.regionServiceURLs {
			knownRegions = append(knownRegions, r)
		}
		missingServiceTypes, possibleRegions = c.possibleRegions([]string{})
		errorPrefix = fmt.Sprintf("invalid region %q", c.creds.Region)
	} else {
		existingServiceTypes := []string{}
		for serviceType := range serviceURLs {
			if containsString(c.requiredServiceTypes, serviceType) {
				existingServiceTypes = append(existingServiceTypes, serviceType)
			}
		}
		missingServiceTypes, possibleRegions = c.possibleRegions(existingServiceTypes)
		errorPrefix = fmt.Sprintf("the configured region %q does not allow access to all required services, namely: %s\n"+
			"access to these services is missing: %s",
			c.creds.Region,
			strings.Join(c.requiredServiceTypes, ", "),
			strings.Join(missingServiceTypes, ", "))
	}
	if len(missingServiceTypes) > 0 {
		if len(possibleRegions) > 0 {
			return fmt.Errorf("%s\none of these regions may be suitable instead: %s",
				errorPrefix,
				strings.Join(possibleRegions, ", "))
		} else {
			return errors.New(errorPrefix)
		}
	}
	c.serviceURLs = serviceURLs
	return nil
}

// possibleRegions returns a list of regions, any of which will allow the client to access the required service types.
// This method is called when a client authenticates and the configured region does not allow access to all the
// required service types. The service types which are accessible, accessibleServiceTypes, is passed in and the
// method returns what the missing service types are as well as valid regions.
func (c *authenticatingClient) possibleRegions(accessibleServiceTypes []string) (missingServiceTypes []string, possibleRegions []string) {
	var serviceTypeRegions map[string][]string
	// Figure out the missing service types and build up a map of all service types to regions
	// obtained from the authentication response.
	for _, serviceType := range c.requiredServiceTypes {
		if !containsString(accessibleServiceTypes, serviceType) {
			missingServiceTypes = append(missingServiceTypes, serviceType)
			serviceTypeRegions = c.extractServiceTypeRegions()
		}
	}

	// Look at the region lists for each missing service type and determine which subset of those could
	// be used to allow access to all required service types. The most specific regions are used.
	if len(missingServiceTypes) == 1 {
		possibleRegions = serviceTypeRegions[missingServiceTypes[0]]
	} else {
		for _, serviceType := range missingServiceTypes {
			for _, serviceTypeCompare := range missingServiceTypes {
				if serviceType == serviceTypeCompare {
					continue
				}
				possibleRegions = appendPossibleRegions(serviceType, serviceTypeCompare, serviceTypeRegions, possibleRegions)
			}
		}
	}
	sort.Strings(possibleRegions)
	return
}

// utility function to extract map of service types -> region
func (c *authenticatingClient) extractServiceTypeRegions() map[string][]string {
	serviceTypeRegions := make(map[string][]string)
	for region, serviceURLs := range c.regionServiceURLs {
		for regionServiceType := range serviceURLs {
			regions := serviceTypeRegions[regionServiceType]
			if !containsString(regions, region) {
				serviceTypeRegions[regionServiceType] = append(regions, region)
			}
		}
	}
	return serviceTypeRegions
}

// extract the common regions for each service type and append them to the possible regions slice.
func appendPossibleRegions(serviceType, serviceTypeCompare string, serviceTypeRegions map[string][]string,
	possibleRegions []string) []string {
	regions := serviceTypeRegions[serviceType]
	for _, region := range regions {
		regionsCompare := serviceTypeRegions[serviceTypeCompare]
		if !containsBaseRegion(regionsCompare, region) && containsSuperRegion(regionsCompare, region) {
			possibleRegions = append(possibleRegions, region)
		}
	}
	return possibleRegions
}

// utility function to see if element exists in values slice.
func containsString(values []string, element string) bool {
	for _, value := range values {
		if value == element {
			return true
		}
	}
	return false
}

// containsBaseRegion returns true if any of the regions in values are based on region.
// see client.regionMatches.
func containsBaseRegion(values []string, region string) bool {
	for _, value := range values {
		if regionMatches(value, region) && region != value {
			return true
		}
	}
	return false
}

// containsSuperRegion returns true if region is based on any of the regions in values.
// see client.regionMatches.
func containsSuperRegion(values []string, region string) bool {
	for _, value := range values {
		if regionMatches(region, value) || region == value {
			return true
		}
	}
	return false
}

func regionMatches(userRegion, endpointRegion string) bool {
	// The user specified region (from the credentials config) matches
	// the endpoint region if the user region equals or ends with the endpoint region.
	// eg  user region "az-1.region-a.geo-1" matches endpoint region "region-a.geo-1"
	return strings.HasSuffix(userRegion, endpointRegion)
}

func (c *authenticatingClient) setToken(tokenId string) {
	c.mu.Lock()
	c.tokenId = tokenId
	c.mu.Unlock()
}

func (c *authenticatingClient) Token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokenId
}

func (c *authenticatingClient) UserId() string {
	return c.userId
}

func (c *authenticatingClient) TenantId() string {
	return c.tenantId
}

func (c *authenticatingClient) IsAuthenticated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokenId != ""
}

var authenticationTimeout = time.Duration(60) * time.Second

func (c *authenticatingClient) Authenticate() (err error) {
	ok := goosesync.RunWithTimeout(authenticationTimeout, func() {
		err = c.doAuthenticate()
	})
	if !ok {
		err = gooseerrors.NewTimeoutf(
			nil, "", "Authentication response not received in %s.", authenticationTimeout)
	}
	return err
}

func (c *authenticatingClient) doAuthenticate() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.creds == nil || c.tokenId != "" {
		return nil
	}
	if c.authMode == nil {
		return fmt.Errorf("Authentication method has not been specified")
	}
	var (
		authDetails *identity.AuthDetails
		err         error
	)
	if authDetails, err = c.authMode.Auth(c.creds); err != nil {
		return gooseerrors.Newf(err, "authentication failed")
	}
	c.regionServiceURLs = authDetails.RegionServiceURLs
	if err := c.createServiceURLs(); err != nil {
		return gooseerrors.Newf(err, "cannot create service URLs")
	}
	c.tenantId = authDetails.TenantId
	c.userId = authDetails.UserId
	// A valid token indicates authorisation has been successful, so it needs to be set last. It must be set
	// after the service URLs have been extracted.
	c.tokenId = authDetails.Token
	return nil
}

func (c *authenticatingClient) IdentityAuthOptions() (identity.AuthOptions, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.authOptions) > 0 {
		return c.authOptions, nil
	}
	baseURL := c.baseURL
	if baseURL == "" {
		parsedURL, err := url.Parse(c.creds.URL)
		if err != nil {
			return identity.AuthOptions{}, gooseerrors.Newf(err, "trying to determine auth information url")
		}
		// this cannot fail.
		authInfoPath, _ := url.Parse("/")
		baseURL = parsedURL.ResolveReference(authInfoPath).String()
	}
	authOptions, err := identity.FetchAuthOptions(baseURL, c.httpClient, c.logger)
	if err != nil {
		return identity.AuthOptions{}, gooseerrors.Newf(err, "auth options fetching failed")
	}
	c.authOptions = authOptions
	return authOptions, nil
}
