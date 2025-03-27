// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package snap

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"time"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/internal/errors"
)

// LookupAssertions attempts to download an assertion list from the snap store
// proxy located at proxyURL and locate the store ID associated with the
// specified proxyURL.
//
// If the local snap store proxy instance is operating in an air-gapped
// environment, downloading the assertion list from the proxy will not be
// possible and an appropriate error will be returned.
func LookupAssertions(proxyURL string) (assertions, storeID string, err error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return "", "", errors.Errorf("proxy URL not valid: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", "", errors.Errorf("proxy URL scheme %q %w", u.Scheme, coreerrors.NotValid)
	}

	// Make sure to redact user/pass when including the proxy URL in error messages
	u.User = nil
	noCredsProxyURL := u.String()

	pathURL, _ := url.Parse("/v2/auth/store/assertions")
	req, _ := http.NewRequest("GET", u.ResolveReference(pathURL).String(), nil)
	ctx, cancelFn := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelFn()

	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		return "", "", errors.Errorf("could not contact snap store proxy at %q. If using an air-gapped proxy you must manually provide the assertions file and store ID: %w", noCredsProxyURL, err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return "", "", errors.Errorf("could not retrieve assertions from proxy at %q; proxy replied with unexpected HTTP status code %d", noCredsProxyURL, res.StatusCode)
	}

	data, err := io.ReadAll(res.Body)
	if err != nil {
		return "", "", errors.Errorf("could not read assertions response from proxy at %q: %w", noCredsProxyURL, err)
	}
	assertions = string(data)
	if storeID, err = findStoreID(assertions, u); err != nil {
		return "", "", errors.Capture(err)
	}

	return assertions, storeID, nil
}

var storeInAssertionRE = regexp.MustCompile(`(?is)type: store.*?store: ([a-zA-Z0-9]+).*?url: (https?://[^\s]+)`)

func findStoreID(assertions string, proxyURL *url.URL) (string, error) {
	var storeID string
	for _, match := range storeInAssertionRE.FindAllStringSubmatch(assertions, -1) {
		if len(match) != 3 {
			continue
		}

		// Found store assertion but not for the URL provided
		storeURL, err := url.Parse(match[2])
		if err != nil {
			continue
		}
		if storeURL.Host != proxyURL.Host {
			continue
		}

		// Found same URL but different store ID
		if storeID != "" && match[1] != storeID {
			return "", errors.Errorf("assertions response from proxy at %q is ambiguous as it contains multiple entries with the same proxy URL but different store ID", proxyURL)
		}

		storeID = match[1]
	}

	if storeID == "" {
		return "", errors.Errorf("store ID in assertions response from proxy at %q %w", proxyURL, coreerrors.NotFound)
	}

	return storeID, nil
}
