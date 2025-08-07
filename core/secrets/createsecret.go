// Copyright 2021 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package secrets

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/juju/utils/v4"
	"gopkg.in/yaml.v2"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

var keyRegExp = regexp.MustCompile("^([a-z](?:-?[a-z0-9]){2,})$")

// SecretData holds secret key values.
type SecretData map[string]string

const (
	fileSuffix = "#file"
	// Ideally we'd use 1MiB as that's what k8s supports, but vault
	// accepts slightly less so we punt on 1MB which will be supported
	// everywhere we care about.
	maxValueSizeBytes   = 1000 * 1000
	maxContentSizeBytes = 1000 * 1000
)

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
			return nil, errors.Errorf("key value %q %w", val, coreerrors.NotValid)
		}
		keyVal := []string{
			val[0:idx],
			val[idx+1:],
		}
		key := keyVal[0]
		value := keyVal[1]
		if !strings.HasSuffix(key, fileSuffix) {
			data[key] = value
			continue
		}
		key = strings.TrimSuffix(key, fileSuffix)
		path, err := utils.NormalizePath(value)
		if err != nil {
			return nil, errors.Capture(err)
		}
		fs, err := os.Stat(path)
		if err == nil && fs.Size() > maxValueSizeBytes {
			return nil, errors.Errorf("secret content in file %q too large: %d bytes", path, fs.Size())
		}
		content, err := os.ReadFile(value)
		if err != nil {
			return nil, errors.Errorf("reading content for secret key %q: %w", key, err)
		}
		data[key] = string(content)
	}
	return encodeBase64(data)
}

// ReadSecretData reads secret data from a YAML or JSON file as key value pairs.
func ReadSecretData(f string) (SecretData, error) {
	attrs := make(SecretData)
	path, err := utils.NormalizePath(f)
	if err != nil {
		return nil, errors.Capture(err)
	}
	fs, err := os.Stat(path)
	if err == nil && fs.Size() > maxContentSizeBytes {
		return nil, errors.Errorf("secret content in file %q too large: %d bytes", path, fs.Size())
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.Capture(err)
	}
	if err := json.Unmarshal(data, &attrs); err != nil {
		err = yaml.Unmarshal(data, &attrs)
		if err != nil {
			return nil, errors.Capture(err)
		}
	}
	return encodeBase64(attrs)
}

const base64Suffix = "#base64"

func encodeBase64(in SecretData) (SecretData, error) {
	out := make(SecretData, len(in))
	var contentSize int
	for key, value := range in {
		if strings.HasSuffix(key, base64Suffix) {
			key = strings.TrimSuffix(key, base64Suffix)
		} else {
			value = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%v", value)))
		}
		if !keyRegExp.MatchString(key) {
			return nil, errors.Errorf("key %q %w", key, coreerrors.NotValid)
		}
		valSize := len(value)
		if valSize > maxValueSizeBytes {
			return nil, errors.Errorf("base64 encoded secret content for key %q too large: %d bytes", key, valSize)
		}
		out[key] = value
		contentSize += valSize
	}
	if contentSize > maxContentSizeBytes {
		return nil, errors.Errorf("base64 encoded secret content too large: %d bytes", contentSize)
	}
	return out, nil
}
