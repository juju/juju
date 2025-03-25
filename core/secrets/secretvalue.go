// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// SecretValue holds the value of a secret.
// Instances of SecretValue are returned by a secret store
// when a secret look up is performed. The underlying value
// is a map of base64 encoded values represented as []byte.
type SecretValue interface {
	// EncodedValues returns the key values of a secret as
	// the raw base64 encoded strings.
	// For the special case where the secret only has a
	// single key value "data", then use BinaryValue()
	//to get the result.
	EncodedValues() map[string]string

	// Values returns the key values of a secret as strings.
	// For the special case where the secret only has a
	// single key value "data", then use StringValue()
	//to get the result.
	Values() (map[string]string, error)

	// KeyValue returns the specified secret value for the key.
	// If the key has a #base64 suffix, the returned value is base64 encoded.
	KeyValue(string) (string, error)

	// IsEmpty checks if the value is empty.
	IsEmpty() bool

	// Checksum is the checksum of the secret content.
	Checksum() (string, error)
}

type secretValue struct {
	// Data holds the key values of a secret.
	// We use a map to hold multiple values, eg cert and key
	// The serialised form of any string values is a
	// base64 encoded string, representing arbitrary values.
	data map[string][]byte
}

// NewSecretValue returns a secret using the specified map of values.
// The map values are assumed to be already base64 encoded.
func NewSecretValue(data map[string]string) SecretValue {
	dataCopy := make(map[string][]byte, len(data))
	for k, v := range data {
		dataCopy[k] = append([]byte(nil), v...)
	}
	return &secretValue{data: dataCopy}
}

// NewSecretBytes returns a secret using the specified map of values.
// The map values are assumed to be already base64 encoded.
func NewSecretBytes(data map[string][]byte) SecretValue {
	dataCopy := make(map[string][]byte, len(data))
	for k, v := range data {
		dataCopy[k] = append([]byte(nil), v...)
	}
	return &secretValue{data: dataCopy}
}

// IsEmpty checks if the value is empty.
func (v secretValue) IsEmpty() bool {
	return len(v.data) == 0
}

// EncodedValues implements SecretValue.
func (v secretValue) EncodedValues() map[string]string {
	dataCopy := make(map[string]string, len(v.data))
	for k, val := range v.data {
		dataCopy[k] = string(val)
	}
	return dataCopy
}

// Values implements SecretValue.
func (v secretValue) Values() (map[string]string, error) {
	dataCopy := v.EncodedValues()
	for k, v := range dataCopy {
		data, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, errors.Capture(err)
		}
		dataCopy[k] = string(data)
	}
	return dataCopy, nil
}

// KeyValue implements SecretValue.
func (v secretValue) KeyValue(key string) (string, error) {
	useBase64 := false
	if strings.HasSuffix(key, base64Suffix) {
		key = strings.TrimSuffix(key, base64Suffix)
		useBase64 = true
	}
	val, ok := v.data[key]
	if !ok {
		return "", errors.Errorf("secret key value %q %w", key, coreerrors.NotFound)
	}
	// The stored value is always base64 encoded.
	if useBase64 {
		return string(val), nil
	}
	b64 := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(val))
	result, err := io.ReadAll(b64)
	if err != nil {
		return "", errors.Capture(err)
	}
	return string(result), nil
}

// Checksum implements SecretValue.
func (v secretValue) Checksum() (string, error) {
	data, err := json.Marshal(v.EncodedValues())
	if err != nil {
		return "", errors.Capture(err)
	}
	hash := sha256.New()
	_, err = hash.Write(data)
	if err != nil {
		return "", errors.Capture(err)
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
