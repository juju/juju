// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"encoding/base64"
	"fmt"
	"strings"

	"github.com/juju/errors"
)

// SecretData holds secret key values.
type SecretData map[string][]byte

// CreatSecretData creates a secret data bag from a list of arguments.
// The arguments are either all key=value or a singular value.
// If base64 is true, then the supplied value(s) are already base64 encoded,
// otherwise the values are base64 encoded as they are added to the data bag.
func CreatSecretData(asBase64 bool, args []string) (SecretData, error) {
	data := make(SecretData)
	haveSingularalue := false
	for i, val := range args {
		// Remove any base64 padding ("=") before splitting the key=value.
		stripped := strings.TrimRight(val, string(base64.StdPadding))
		idx := strings.Index(stripped, "=")
		keyVal := []string{val}
		if idx > 0 {
			keyVal = []string{
				val[0:idx],
				val[idx+1:],
			}
		}

		var (
			key   string
			value string
		)
		if len(keyVal) == 1 {
			if i > 0 {
				return nil, errors.NewNotValid(nil, fmt.Sprintf("singular value %q not valid when other key values are specified", val))
			}
			key = singularSecretKey
			value = keyVal[0]
			haveSingularalue = true
		} else {
			key = keyVal[0]
			value = keyVal[1]
		}
		if haveSingularalue && i > 0 {
			return nil, errors.NewNotValid(nil, fmt.Sprintf("key value %q not valid when a singular value has already been specified", val))
		}
		if !asBase64 {
			value = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", value)))
		}
		data[key] = []byte(value)
	}
	return data, nil
}
