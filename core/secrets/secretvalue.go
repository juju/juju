// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"bytes"
	"encoding/base64"
	"io"
	"strings"

	"github.com/juju/errors"
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
func (v *secretValue) EncodedValues() map[string]string {
	dataCopy := make(map[string]string, len(v.data))
	for k, val := range v.data {
		dataCopy[k] = string(val)
	}
	return dataCopy
}

// Values implements SecretValue.
func (v *secretValue) Values() (map[string]string, error) {
	dataCopy := v.EncodedValues()
	for k, v := range dataCopy {
		data, err := base64.StdEncoding.DecodeString(v)
		if err != nil {
			return nil, errors.Trace(err)
		}
		dataCopy[k] = string(data)
	}
	return dataCopy, nil
}

// KeyValue implements SecretValue.
func (v *secretValue) KeyValue(key string) (string, error) {
	useBase64 := false
	if strings.HasSuffix(key, base64Suffix) {
		key = strings.TrimSuffix(key, base64Suffix)
		useBase64 = true
	}
	val, ok := v.data[key]
	if !ok {
		return "", errors.NotFoundf("secret key value %q", key)
	}
	// The stored value is always base64 encoded.
	if useBase64 {
		return string(val), nil
	}
	b64 := base64.NewDecoder(base64.StdEncoding, bytes.NewReader(val))
	result, err := io.ReadAll(b64)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(result), nil
}
