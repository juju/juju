// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// CreateSecretArgs holds args for creating secrets.
type CreateSecretArgs struct {
	Args []CreateSecretArg `json:"args"`
}

// CreateSecretArg holds the args for creating a secret.
type CreateSecretArg struct {
	// Type is "blob" for secrets where the data is passed
	// in here; "password" is use for where the actual
	// value is generated server side using arguments
	// in Params.
	Type string `json:"type"`
	// Path represents a unique string used to identify a secret.
	Path string `json:"path"`
	// Scope defines the context in which a secret can be used.
	// eg "application" or "model".
	Scope string `json:"scope"`
	// Params are used when generating secrets server side.
	// See core/secrets/secret.go.
	Params map[string]interface{} `json:"params,omitempty"`
	// Data is the key values of the secret value itself.
	Data map[string]string `json:"data,omitempty"`
}

// GetSecretArgs holds the args for getting secrets.
type GetSecretArgs struct {
	Args []GetSecretArg `json:"args"`
}

// GetSecretArg holds the args for getting a secret.
type GetSecretArg struct {
	ID string `json:"id"`
}

// SecretValueResults holds secret value results.
type SecretValueResults struct {
	Results []SecretValueResult `json:"results"`
}

// SecretValueResult is the result of getting a secret value.
type SecretValueResult struct {
	Data  map[string]string `json:"data"`
	Error *Error            `json:"error,omitempty"`
}
