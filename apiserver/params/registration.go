// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

// SecretKeyLoginRequest contains the parameters for completing
// the registration of a user. The request contains the tag of
// the user, and an encrypted and authenticated payload that
// proves that the requester has a secret key recorded on the
// controller.
type SecretKeyLoginRequest struct {
	// User is the tag-representation of the user that the
	// requester wishes to authenticate as.
	User string `json:"user"`

	// Nonce is the nonce used by the client to encrypt
	// and authenticate PayloadCiphertext.
	Nonce []byte `json:"nonce"`

	// PayloadCiphertext is the encrypted and authenticated
	// payload. The payload is encrypted/authenticated using
	// NaCl Secretbox.
	PayloadCiphertext []byte `json:"ciphertext"`
}

// SecretKeyLoginRequestPayload is JSON-encoded and then encrypted
// and authenticated with the NaCl Secretbox algorithm.
type SecretKeyLoginRequestPayload struct {
	// Password is the new password to set for the user.
	Password string `json:"password"`
}

// SecretKeyLoginResponse contains the result of completing a user
// registration. This contains an encrypted and authenticated payload,
// containing the information necessary to securely log into the
// controller via the standard password authentication method.
type SecretKeyLoginResponse struct {
	// Nonce is the nonce used by the server to encrypt and
	// authenticate PayloadCiphertext.
	Nonce []byte `json:"nonce"`

	// PayloadCiphertext is the encrypted and authenticated
	// payload, which is a JSON-encoded SecretKeyLoginResponsePayload.
	PayloadCiphertext []byte `json:"ciphertext"`
}

// SecretKeyLoginResponsePayload is JSON-encoded and then encrypted
// and authenticated with the NaCl Secretbox algorithm.
type SecretKeyLoginResponsePayload struct {
	// CACert is the CA certificate, required to establish a secure
	// TLS connection to the Juju controller
	CACert string `json:"ca-cert"`

	// ControllerUUID is the UUID of the Juju controller.
	ControllerUUID string `json:"controller-uuid"`
}
