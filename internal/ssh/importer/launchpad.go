// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/errors"
)

// LaunchpadResolver is an implementation of [Resolver] for retrieving the
// public keys of a Launchpad user.
type LaunchpadResolver struct {
	// Client is the http client to use for talking with Launchpad.
	Client Client
}

const (
	// launchpadURL is the constant url for contacting Launchpad.
	launchpadURL = "https://launchpad.net"

	// launchpadPathUserKeys is the Launchpad path for fetching a users public
	// keys.
	launchpadPathUserKeys = "%s/+sshkeys"
)

// PublicKeysForSubject implements the [Resolver] interface by taking a
// Launchpad subject in this case a user and returning all of the public ssh
// keys the user has for their profile.
// The following errors can be expected:
// - [SubjectNotFound] when the subject being asked for does not exist in
// the resolvers domain.
func (l *LaunchpadResolver) PublicKeysForSubject(
	ctx context.Context,
	subject string,
) ([]string, error) {
	url, err := url.Parse(launchpadPathUserKeys)
	if err != nil {
		return nil, fmt.Errorf("parsing launchpad url %q: %w", launchpadURL, err)
	}

	url = url.JoinPath(fmt.Sprintf(launchpadPathUserKeys, subject))

	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf(
			"generating launchpad user keys request for user %q: %w",
			subject, err,
		)
	}

	req = setAccept(req, contentTypeTextUTF8)
	res, err := l.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"getting launchpad authorised keys for user %q: %w", subject, err,
		)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf(
			"cannot find launchpad user %q%w",
			subject, errors.Hide(SubjectNotFound),
		)
	} else if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"cannot get public keys for launchpad user %q, recieved status %q",
			subject, res.Status,
		)
	}

	safeEncoding, err := hasContentType(res, contentTypeTextUTF8)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot determine content type of launchpad public keys response for user %q: %w",
			subject, err,
		)
	}

	if !safeEncoding {
		return nil, fmt.Errorf(
			"launchpad public keys for user %q returned an unknown encoding for response",
			subject,
		)
	}

	rval := make([]string, 0)
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		line := scanner.Text()
		// If the line is empty we can skip it.
		if line == "" {
			continue
		}
		rval = append(rval, line)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf(
			"decoding launchpad user %q public keys: %w",
			subject, err,
		)
	}

	return rval, nil
}
