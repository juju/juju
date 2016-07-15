// goose/identity - Go package to interact with OpenStack Identity (Keystone) API.

package identity

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"reflect"
	"strings"

	goosehttp "gopkg.in/goose.v1/http"
)

// AuthMode defines the authentication method to use (see Auth*
// constants below).
type AuthMode int

const (
	AuthLegacy     = AuthMode(iota) // Legacy authentication
	AuthUserPass                    // Username + password authentication
	AuthKeyPair                     // Access/secret key pair authentication
	AuthUserPassV3                  // Username + password authentication (v3 API)
)

func (a AuthMode) String() string {
	switch a {
	case AuthKeyPair:
		return "Access/Secret Key Authentication"
	case AuthLegacy:
		return "Legacy Authentication"
	case AuthUserPass:
		return "Username/password Authentication"
	case AuthUserPassV3:
		return "Username/password Authentication (Version 3)"
	}
	panic(fmt.Errorf("Unknown athentication type: %d", a))
}

type AuthOption struct {
	Mode     AuthMode
	Endpoint string
}

type AuthOptions []AuthOption

type ServiceURLs map[string]string

// AuthDetails defines all the necessary information, needed for an
// authenticated session with OpenStack.
type AuthDetails struct {
	Token             string
	TenantId          string
	UserId            string
	RegionServiceURLs map[string]ServiceURLs // Service type to endpoint URLs for each region
}

// Credentials defines necessary parameters for authentication.
type Credentials struct {
	URL        string // The URL to authenticate against
	User       string // The username to authenticate as
	Secrets    string // The secrets to pass
	Region     string // Region to send requests to
	TenantName string // The tenant information for this connection
	DomainName string `credentials:"optional"` // The domain for this user (new in keystone v3)
}

// Authenticator is implemented by each authentication method.
type Authenticator interface {
	Auth(creds *Credentials) (*AuthDetails, error)
}

// getConfig returns the value of the first available environment
// variable, among the given ones.
func getConfig(envVars []string) (value string) {
	value = ""
	for _, v := range envVars {
		value = os.Getenv(v)
		if value != "" {
			break
		}
	}
	return
}

// The following variables hold the names of environment variables
// that are used by CredentialsFromEnv to populate a Credentials
// value. The most preferred names are at the start of the slices.
var (
	// CredEnvAuthURL is used for Credentials.URL.
	CredEnvAuthURL = []string{
		"OS_AUTH_URL",
	}
	// CredEnvUser is used for Credentials.User.
	CredEnvUser = []string{
		"OS_USERNAME",
		"NOVA_USERNAME",
		"OS_ACCESS_KEY",
		"NOVA_API_KEY",
	}
	// CredEnvSecrets is used for Credentials.Secrets.
	CredEnvSecrets = []string{
		"OS_PASSWORD",
		"NOVA_PASSWORD",
		"OS_SECRET_KEY",
		// Apparently some clients did use this.
		"AWS_SECRET_ACCESS_KEY",
		// This is manifestly a misspelling but we leave
		// it here just in case anyone did actually use it.
		"EC2_SECRET_KEYS",
	}
	// CredEnvRegion is used for Credentials.Region.
	CredEnvRegion = []string{
		"OS_REGION_NAME",
		"NOVA_REGION",
	}
	// CredEnvTenantName is used for Credentials.TenantName.
	CredEnvTenantName = []string{
		"OS_TENANT_NAME",
		"NOVA_PROJECT_ID",
	}
	// CredEnvDomainName is used for Credentials.TenantName.
	CredEnvDomainName = []string{
		"OS_DOMAIN_NAME",
	}
)

// CredentialsFromEnv creates and initializes the credentials from the
// environment variables.
func CredentialsFromEnv() *Credentials {
	return &Credentials{
		URL:        getConfig(CredEnvAuthURL),
		User:       getConfig(CredEnvUser),
		Secrets:    getConfig(CredEnvSecrets),
		Region:     getConfig(CredEnvRegion),
		TenantName: getConfig(CredEnvTenantName),
		DomainName: getConfig(CredEnvDomainName),
	}
}

// CompleteCredentialsFromEnv gets and verifies all the required
// authentication parameters have values in the environment.
func CompleteCredentialsFromEnv() (cred *Credentials, err error) {
	cred = CredentialsFromEnv()
	v := reflect.ValueOf(cred).Elem()
	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		tag := t.Field(i).Tag.Get("credentials")
		if f.String() == "" && tag != "optional" {
			err = fmt.Errorf("required environment variable not set for credentials attribute: %s", t.Field(i).Name)
		}
	}
	return
}

// NewAuthenticator creates an authenticator matching the supplied AuthMode.
// The httpclient is allowed to be nil, the Authenticator will just use the
// default http.Client
func NewAuthenticator(authMode AuthMode, httpClient *goosehttp.Client) Authenticator {
	if httpClient == nil {
		httpClient = goosehttp.New()
	}
	switch authMode {
	default:
		panic(fmt.Errorf("Invalid identity authorisation mode: %d", authMode))
	case AuthLegacy:
		return &Legacy{client: httpClient}
	case AuthUserPass:
		return &UserPass{client: httpClient}
	case AuthKeyPair:
		return &KeyPair{client: httpClient}
	case AuthUserPassV3:
		return &V3UserPass{client: httpClient}
	}
}

type authInformationLink struct {
	Href string `json:"href"`
	Rel  string `json:"base"`
}

type authInformationMediaType struct {
	Base      string `json:"base"`
	MediaType string `json:"type"`
}

type authInformationValue struct {
	ID         string                     `json:"id"`
	Links      []authInformationLink      `json:"links"`
	MediaTypes []authInformationMediaType `json:"media-types"`
	Status     string                     `json:"status"`
	Updated    string                     `json:"updates"`
}

type authInformationVersions struct {
	Values []authInformationValue `json:"values"`
}

type authInformation struct {
	Versions authInformationVersions `json:"versions"`
}

// FetchAuthOptions returns the authentication options advertised by this
// openstack.
func FetchAuthOptions(url string, client *goosehttp.Client, logger *log.Logger) (AuthOptions, error) {
	var resp authInformation
	req := goosehttp.RequestData{
		RespValue: &resp,
		ExpectedStatus: []int{
			http.StatusMultipleChoices,
		},
	}
	if err := client.JsonRequest("GET", url, "", &req, nil); err != nil {
		return nil, fmt.Errorf("request available auth options: %v", err)
	}
	var auths AuthOptions
	if len(resp.Versions.Values) > 0 {
		for _, version := range resp.Versions.Values {
			// TODO(perrito666) figure more cases.
			link := ""
			if len(version.Links) > 0 {
				link = version.Links[0].Href
			}
			var opt AuthOption
			switch {
			case strings.HasPrefix(version.ID, "v3"):
				opt = AuthOption{Mode: AuthUserPassV3, Endpoint: link}
			case strings.HasPrefix(version.ID, "v2"):
				opt = AuthOption{Mode: AuthUserPass, Endpoint: link}
			default:
				logger.Printf("Unknown authentication version %q\n", version.ID)
			}
			auths = append(auths, opt)
		}
	}
	return auths, nil
}
