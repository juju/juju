package identity

import (
	"fmt"
	"net/http"
	"time"

	goosehttp "gopkg.in/goose.v1/http"
)

// v3AuthWrapper wraps the v3AuthRequest to perform v3 authentication.
type v3AuthWrapper struct {
	Auth v3AuthRequest `json:"auth"`
}

// v3AuthRequest contains the authentication request.
type v3AuthRequest struct {
	Identity v3AuthIdentity `json:"identity"`
	Scope    *v3AuthScope   `json:"scope,omitempty"`
}

// v3AuthIdentity contains the identity portion of an authentication
// request.
type v3AuthIdentity struct {
	Methods  []string        `json:"methods"`
	Password *v3AuthPassword `json:"password,omitempty"`
	Token    *v3AuthToken    `json:"token,omitempty"`
}

// v3AuthPassword contains a password authentication request.
type v3AuthPassword struct {
	User v3AuthUser `json:"user"`
}

// v3AuthUser contains the user part of a password authentication
// request..
type v3AuthUser struct {
	Domain   *v3AuthDomain `json:"domain,omitempty"`
	ID       string        `json:"id,omitempty"`
	Name     string        `json:"name,omitempty"`
	Password string        `json:"password"`
}

// v3AuthDomain contains a domain definition of an authentication
// request.
type v3AuthDomain struct {
	ID   string `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

// v3AuthToken contains the token to use for token authentication.
type v3AuthToken struct {
	ID string `json:"id"`
}

// v3AuthScope contains the scope of the authentication request.
type v3AuthScope struct {
	Domain  *v3AuthDomain  `json:"domain,omitempty"`
	Project *v3AuthProject `json:"project,omitempty"`
}

// v3AuthProject contains the project scope for the authentication
// request.
type v3AuthProject struct {
	Domain *v3AuthDomain `json:"domain,omitempty"`
	ID     string        `json:"id,omitempty"`
	Name   string        `json:"name,omitempty"`
}

// V3UserPass is an Authenticator that will perform username + password
// authentication using the v3 protocol.
type V3UserPass struct {
	client *goosehttp.Client
}

// Auth performs a v3 username + password authentication request using
// the values supplied in creds.
func (u *V3UserPass) Auth(creds *Credentials) (*AuthDetails, error) {
	if u.client == nil {
		u.client = goosehttp.New()
	}
	domain := creds.DomainName
	if domain == "" {
		domain = "default"
	}
	auth := v3AuthWrapper{
		Auth: v3AuthRequest{
			Identity: v3AuthIdentity{
				Methods: []string{"password"},
				Password: &v3AuthPassword{
					User: v3AuthUser{
						Domain: &v3AuthDomain{
							Name: domain,
						},
						Name:     creds.User,
						Password: creds.Secrets,
					},
				},
			},
		},
	}
	if creds.TenantName != "" {
		auth.Auth.Scope = &v3AuthScope{
			Project: &v3AuthProject{
				Domain: &v3AuthDomain{
					Name: domain,
				},
				Name: creds.TenantName,
			},
		}
	}
	// TODO(perrito666) it is possible to scope by domain too
	// but we don't have yet a clear way to add this into the
	// credentials.

	return v3KeystoneAuth(u.client, &auth, creds.URL)
}

type v3TokenWrapper struct {
	Token v3Token `json:"token"`
}

// v3Token represents the reponse token as described in:
// http://developer.openstack.org/api-ref-identity-v3.html#authenticatePasswordScoped
type v3Token struct {
	Expires time.Time        `json:"expires_at"`
	Issued  time.Time        `json:"issued_at"`
	Methods []string         `json:"methods"`
	Catalog []v3TokenCatalog `json:"catalog"`
	Project v3TokenProject   `json:"project"`
	User    v3TokenUser      `json:"user"`
}

type v3TokenCatalog struct {
	ID        string            `json:"id"`
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Endpoints []v3TokenEndpoint `json:"endpoints"`
}

type v3TokenEndpoint struct {
	ID        string `json:"id"`
	RegionID  string `json:"region_id"`
	URL       string `json:"url"`
	Interface string `json:"interface"`
}

type v3TokenProject struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Domain v3TokenDomain `json:"domain"`
}

type v3TokenUser struct {
	ID     string        `json:"id"`
	Name   string        `json:"name"`
	Domain v3TokenDomain `json:"domain"`
}

type v3TokenDomain struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// v3KeystoneAuth performs a v3 authentication request.
func v3KeystoneAuth(c *goosehttp.Client, v interface{}, url string) (*AuthDetails, error) {
	var resp v3TokenWrapper
	req := goosehttp.RequestData{
		ReqValue:  v,
		RespValue: &resp,
		ExpectedStatus: []int{
			http.StatusCreated,
		},
	}
	if err := c.JsonRequest("POST", url, "", &req, nil); err != nil {
		return nil, fmt.Errorf("requesting token: %v", err)
	}
	tok := req.RespHeaders.Get("X-Subject-Token")
	if tok == "" {
		return nil, fmt.Errorf("authentication failed")
	}
	rsu := make(map[string]ServiceURLs, len(resp.Token.Catalog))
	for _, s := range resp.Token.Catalog {
		for _, ep := range s.Endpoints {
			if ep.Interface != "public" {
				continue
			}
			su, ok := rsu[ep.RegionID]
			if !ok {
				su = make(ServiceURLs)
				rsu[ep.RegionID] = su
			}
			su[s.Type] = ep.URL
		}
	}
	return &AuthDetails{
		Token:             tok,
		TenantId:          resp.Token.Project.ID,
		UserId:            resp.Token.User.ID,
		RegionServiceURLs: rsu,
	}, nil
}
