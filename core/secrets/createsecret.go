// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/utils/v3"
	"gopkg.in/yaml.v2"
)

var keyRegExp = regexp.MustCompile("^([a-z](?:-?[a-z0-9]){2,})$")

// SecretData holds secret key values.
type SecretData map[string]string

// CreateSecretData creates a secret data bag from a list of arguments.
// If a key has the #base64 suffix, then the value is already base64 encoded,
// otherwise the value is base64 encoded as it is added to the data bag.
func CreateSecretData(args []string) (SecretData, error) {
	data := make(SecretData)
	for _, val := range args {
		// Remove any base64 padding ("=") before splitting the key=value.
		stripped := strings.TrimRight(val, string(base64.StdPadding))
		idx := strings.Index(stripped, "=")
		if idx < 1 {
			return nil, errors.NotValidf("key value %q", val)
		}
		keyVal := []string{
			val[0:idx],
			val[idx+1:],
		}
		key := keyVal[0]
		value := keyVal[1]
		data[key] = value
	}
	return encodeBase64(data)
}

// ReadSecretData reads secret data from a YAML or JSON file as key value pairs.
func ReadSecretData(f string) (SecretData, error) {
	attrs := make(SecretData)
	path, err := utils.NormalizePath(f)
	if err != nil {
		return nil, errors.Trace(err)
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, errors.Trace(err)
	}
	if err := json.Unmarshal(data, &attrs); err != nil {
		err = yaml.Unmarshal(data, &attrs)
		if err != nil {
			return nil, errors.Trace(err)
		}
	}
	return encodeBase64(attrs)
}

const base64Suffix = "#base64"

func encodeBase64(in SecretData) (SecretData, error) {
	out := make(SecretData, len(in))
	for k, v := range in {
		if strings.HasSuffix(k, base64Suffix) {
			k = strings.TrimSuffix(k, base64Suffix)
			if !keyRegExp.MatchString(k) {
				return nil, errors.NotValidf("key %q", k)
			}
			out[k] = v
			continue
		}
		if !keyRegExp.MatchString(k) {
			return nil, errors.NotValidf("key %q", k)
		}
		out[k] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", v)))
	}
	return out, nil
}
