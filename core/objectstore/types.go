// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstore

import (
	"fmt"
	"regexp"
	"strings"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// BackendType is the type to identify the backend to use for the object store.
type BackendType string

const (
	// FileBackend is the backend type for the file object store.
	FileBackend BackendType = "file"
	// S3Backend is the backend type for the s3 object store.
	S3Backend BackendType = "s3"
)

func (b BackendType) String() string {
	return string(b)
}

// ParseObjectStoreType parses the given string into a BackendType.
func ParseObjectStoreType(s string) (BackendType, error) {
	switch s {
	case string(FileBackend):
		return FileBackend, nil
	case string(S3Backend):
		return S3Backend, nil
	default:
		return "", errors.Errorf("object store type %q %w", s, coreerrors.NotValid)
	}
}

// BucketName is the name of the bucket to use for the object store.
// See: https://docs.aws.amazon.com/AmazonS3/latest/userguide/bucketnamingrules.html
//
// This function doesn't use one big regexp, as it's harder to update when and
// if they change the naming rules.
func ParseObjectStoreBucketName(s string) (string, error) {
	if s == "" {
		return "", errors.Errorf("bucket name %q %w", s, coreerrors.NotValid)
	}

	// Bucket names must be between 3 (min) and 63 (max) characters long.
	if num := len(s); num < 3 {
		return "", errors.New(fmt.Sprintf("bucket name %q: too short", s)).Add(coreerrors.NotValid)
	} else if num > 63 {
		return "", errors.New(fmt.Sprintf("bucket name %q: too long", s)).Add(coreerrors.NotValid)
	}

	// Bucket names can consist only of lowercase letters, numbers, dots (.),
	// and hyphens (-).
	// Bucket names must begin and end with a letter or number.
	// For best compatibility, we recommend that you avoid using dots (.) in
	// bucket names, except for buckets that are used only for static website
	// hosting. If you include dots in a bucket's name, you can't use
	// virtual-host-style addressing over HTTPS, unless you perform your own
	// certificate validation. This is because the security certificates used
	// for virtual hosting of buckets don't work for buckets with dots in
	// their names.
	if !nameRegex.MatchString(s) {
		return "", errors.New(fmt.Sprintf("bucket name %q: invalid characters", s)).Add(coreerrors.NotValid)
	}

	// Note: We don't allow dots so these test isn't required.
	//  - Bucket names must not contain two adjacent periods (..).
	//  - Bucket names must not be formatted as an IP address (for example, 192.168.5.4).

	// Bucket names must not start with the prefix xn--.
	// Bucket names must not start with the prefix sthree- and the
	// prefix sthree-configurator
	// Note: the later isn't possible because of the last prefix check.
	if strings.HasPrefix(s, "xn--") || strings.HasPrefix(s, "sthree-") {
		return "", errors.New(fmt.Sprintf("bucket name %q: invalid prefix", s)).Add(coreerrors.NotValid)
	}

	// Bucket names must not end with the suffix -s3alias. This suffix is
	// reserved for access point alias names.
	if strings.HasSuffix(s, "-s3alias") {
		return "", errors.New(fmt.Sprintf("bucket name %q: invalid suffix", s)).Add(coreerrors.NotValid)
	}

	return s, nil
}

var (
	// This is the strict regex for bucket names, no dots allowed.
	nameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9\-]{1,61}[a-z0-9]$`)
)
