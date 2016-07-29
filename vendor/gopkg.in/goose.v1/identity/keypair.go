package identity

import (
	goosehttp "gopkg.in/goose.v1/http"
)

// KeyPair allows OpenStack cloud authentication using an access and
// secret key.
//
// It implements Authenticator interface by providing the Auth method.
type KeyPair struct {
	client *goosehttp.Client
}

type keypairCredentials struct {
	AccessKey string `json:"accessKey"`
	SecretKey string `json:"secretKey"`
}

type authKeypairRequest struct {
	KeypairCredentials keypairCredentials `json:"apiAccessKeyCredentials"`
	TenantName         string             `json:"tenantName"`
}

type authKeypairWrapper struct {
	Auth authKeypairRequest `json:"auth"`
}

func (u *KeyPair) Auth(creds *Credentials) (*AuthDetails, error) {
	if u.client == nil {
		u.client = goosehttp.New()
	}
	auth := authKeypairWrapper{Auth: authKeypairRequest{
		KeypairCredentials: keypairCredentials{
			AccessKey: creds.User,
			SecretKey: creds.Secrets,
		},
		TenantName: creds.TenantName}}

	return keystoneAuth(u.client, auth, creds.URL)
}
