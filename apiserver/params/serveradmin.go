package params

// IdentityProviderInfo holds information on a remote identity provider.
type IdentityProviderInfo struct {
	PublicKey string
	Location  string
}

// IdentityProviderResult holds a response containing the remote identity
// provider setting.
type IdentityProviderResult struct {
	IdentityProvider *IdentityProviderInfo
}

// SetIdentityProvider holds the parameters for setting a remote identity
// provider.
type SetIdentityProvider struct {
	IdentityProvider *IdentityProviderInfo
}
