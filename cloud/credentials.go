// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloud

// Credentials is a struct containing cloud credential information.
type Credentials struct {
	// Credentials is a map of cloud credentials, keyed on cloud name.
	Credentials map[string]CloudCredential `yaml:"credentials"`
}

// CloudCredential contains attributes used to define credentials for a cloud.
type CloudCredential struct {
	// DefaultCredential is the named credential to use by default.
	DefaultCredential string `yaml:"default-credential,omitempty"`

	// DefaultRegion is the cloud region to use by default.
	DefaultRegion string `yaml:"default-region,omitempty"`

	// AuthCredentials is the credentials for a cloud, keyed on name.
	AuthCredentials map[string]Credential `yaml:",omitempty,inline"`
}

// Credential instances represent cloud credentials.
type Credential interface {
	// AuthType is the type of authorisation.
	AuthType() AuthType

	// Attributes are the credential values.
	Attributes() map[string]string
}

type cloudCredentialYAML struct {
	CloudCredential `yaml:",inline"`
	AuthCredentials map[string]Credential `yaml:",omitempty,inline"`
}

var _ Credential = (*AccessKeyCredentials)(nil)

// AccessKeyCredentials represent key/secret credentials.
type AccessKeyCredentials struct {
	Key    string `yaml:"key,omitempty"`
	Secret string `yaml:"secret,omitempty"`
}

// AuthType is defined on Credentials interface.
func (c *AccessKeyCredentials) AuthType() AuthType {
	return AccessKeyAuthType
}

// Attributes is defined on Credentials interface.
func (c *AccessKeyCredentials) Attributes() map[string]string {
	return map[string]string{
		"key":    c.Key,
		"secret": c.Secret,
	}
}

var _ Credential = (*OpenstackAccessKeyCredentials)(nil)

// OpenstackAccessKeyCredentials are key/secret credentials for Openstack clouds.
type OpenstackAccessKeyCredentials struct {
	AccessKeyCredentials `yaml:",inline"`
	Tenant               string `yaml:"tenant-name,omitempty"`
}

// AuthType is defined on Credentials interface.
func (c *OpenstackAccessKeyCredentials) AuthType() AuthType {
	return AccessKeyAuthType
}

// Attributes is defined on Credentials interface.
func (c *OpenstackAccessKeyCredentials) Attributes() map[string]string {
	return map[string]string{
		"key":         c.Key,
		"secret":      c.Secret,
		"tenant-name": c.Tenant,
	}
}

// UserPassCredentials are username/password credentials.
type UserPassCredentials struct {
	User     string `yaml:"username,omitempty"`
	Password string `yaml:"password,omitempty"`
}

var _ Credential = (*OpenstackUserPassCredentials)(nil)

// OpenstackUserPassCredentials are user/password credentials for Openstack clouds.
type OpenstackUserPassCredentials struct {
	UserPassCredentials `yaml:",inline"`
	Tenant              string `yaml:"tenant-name,omitempty"`
}

// AuthType is defined on Credentials interface.
func (c *OpenstackUserPassCredentials) AuthType() AuthType {
	return UserPassAuthType
}

// Attributes is defined on Credentials interface.
func (c *OpenstackUserPassCredentials) Attributes() map[string]string {
	return map[string]string{
		"username":    c.User,
		"password":    c.Password,
		"tenant-name": c.Tenant,
	}
}

var _ Credential = (*AzureUserPassCredentials)(nil)

// AzureUserPassCredentials are user/password credentials for Azure clouds.
type AzureUserPassCredentials struct {
	UserPassCredentials `yaml:",inline"`
	ApplicationId       string `yaml:"application-id,omitempty"`
	ApplicationPassword string `yaml:"application-password,omitempty"`
}

// AuthType is defined on Credentials interface.
func (c *AzureUserPassCredentials) AuthType() AuthType {
	return UserPassAuthType
}

// Attributes is defined on Credentials interface.
func (c *AzureUserPassCredentials) Attributes() map[string]string {
	return map[string]string{
		"username":             c.User,
		"password":             c.Password,
		"application-id":       c.ApplicationId,
		"application-password": c.ApplicationPassword,
	}
}

var _ Credential = (*OAuth1Credentials)(nil)

// OAuth1Credentials are oauth1 credentials.
type OAuth1Credentials struct {
	ConsumerKey    string `yaml:"consumer-key,omitempty"`
	ConsumerSecret string `yaml:"consumer-secret,omitempty"`
	AccessToken    string `yaml:"access-token,omitempty"`
	TokenSecret    string `yaml:"token-secret,omitempty"`
}

// AuthType is defined on Credentials interface.
func (c *OAuth1Credentials) AuthType() AuthType {
	return OAuthAuth1Type
}

// Attributes is defined on Credentials interface.
func (c *OAuth1Credentials) Attributes() map[string]string {
	return map[string]string{
		"consumer-key":    c.ConsumerKey,
		"consumer-secret": c.ConsumerSecret,
		"access-token":    c.AccessToken,
		"token-secret":    c.TokenSecret,
	}
}

var _ Credential = (*OAuth2Credentials)(nil)

// OAuth2Credentials are oauth1 credentials.
type OAuth2Credentials struct {
	ClientId    string `yaml:"client-id,omitempty"`
	ClientEmail string `yaml:"client-email,omitempty"`
	PrivateKey  string `yaml:"private-key,omitempty"`
}

// AuthType is defined on Credentials interface.
func (c *OAuth2Credentials) AuthType() AuthType {
	return OAuthAuth2Type
}

// Attributes is defined on Credentials interface.
func (c *OAuth2Credentials) Attributes() map[string]string {
	return map[string]string{
		"client-id":    c.ClientId,
		"client-email": c.ClientEmail,
		"private-key":  c.PrivateKey,
	}
}
