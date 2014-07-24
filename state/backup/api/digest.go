// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package api

import (
	"fmt"
	"net/http"
	"strings"
)

const (
	DigestAlgorithm = "SHA"
)

// ParseDigestHeader returns a map of (algorithm, digest) for all the
// digests found in the "Digest" header.  See RFC 3230.
func ParseDigestHeader(header http.Header) (map[string]string, error) {
	rawdigests := header.Get("digest")
	if rawdigests == "" {
		return nil, fmt.Errorf(`missing or blank "digest" header`)
	}
	digests := make(map[string]string)

	// We do not handle quoted digests that have commas in them.
	for _, rawdigest := range strings.Split(rawdigests, ",") {
		parts := strings.SplitN(rawdigest, "=", 2)
		if len(parts) != 2 {
			return digests, fmt.Errorf(`bad "digest" header: %s`, rawdigest)
		}

		// We do not take care of quoted digests.
		algorithm, digest := parts[0], parts[1]
		if algorithm == "" {
			return digests, fmt.Errorf("missing digest algorithm: %s", rawdigest)
		}
		if digest == "" {
			return digests, fmt.Errorf("missing digest value: %s", rawdigest)
		}

		_, exists := digests[algorithm]
		if exists {
			return digests, fmt.Errorf("duplicate digest: %s", rawdigest)
		}
		digests[algorithm] = digest
	}

	return digests, nil
}

// ExtractSHAFromDigestHeader is a light wrapper around ParseDigestHeader
// which returns just the SHA digest.
func ExtractSHAFromDigestHeader(header http.Header) (string, error) {
	digests, err := ParseDigestHeader(header)
	if err != nil {
		return "", err
	}
	digest, exists := digests[DigestAlgorithm]
	if !exists {
		return "", fmt.Errorf(`"%s" missing from "digest" header`, DigestAlgorithm)
	}
	return digest, nil
}
