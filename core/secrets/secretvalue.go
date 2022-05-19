// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"encoding/base64"
	"io/ioutil"
	"strings"

	"github.com/juju/errors"
)

// SecretValue holds the value of a secret.
// Instances of SecretValue are returned by a secret store
// when a secret look up is performed. The underlying value
// is a map of base64 encoded values represented as []byte.
// Convenience methods exist to retrieve singular decoded string
// and encoded base64 string values.
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

	// Singular returns true if the secret value represents a
	// single data value rather than key values.
	Singular() bool

	// EncodedValue returns the value of the secret as the raw
	// base64 encoded string.
	// The secret must be a singular value.
	EncodedValue() (string, error)

	// Value returns the value of the secret as a string.
	// The secret must be a singular value.
	Value() (string, error)
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

const singularSecretKey = "data"

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

// Singular implements SecretValue.
func (v *secretValue) Singular() bool {
	_, ok := v.data[singularSecretKey]
	return ok && len(v.data) == 1
}

// EncodedValue implements SecretValue.
func (v *secretValue) EncodedValue() (string, error) {
	if !v.Singular() {
		return "", errors.NewNotValid(nil, "secret is not a singular value")
	}
	return string(v.data[singularSecretKey]), nil
}

// Value implements SecretValue.
func (v *secretValue) Value() (string, error) {
	s, err := v.EncodedValue()
	if err != nil {
		return "", errors.Trace(err)
	}
	// The stored value is always base64 encoded.
	b64 := base64.NewDecoder(base64.StdEncoding, strings.NewReader(s))
	result, err := ioutil.ReadAll(b64)
	if err != nil {
		return "", errors.Trace(err)
	}
	return string(result), nil
}
