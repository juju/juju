// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/juju/errors"
)

// githubKeyResponse represents the response returned by Github for fetching a
// users public keys.
type githubKeyResponse struct {
	// Id is internal representation identifier for the key within Github.
	Id int64 `json:"id"`

	// Key represents the public key data for a user's ssh key.
	Key string `json:"key"`
}

// GithubResolver is an implementation of [Resolver] for retrieving the public
// keys of a Github user.
type GithubResolver struct {
	// Client is the http client to use for talking with Github.
	Client Client
}

const (
	// githubContentTypeJSON represents the content type that we accept back
	// from Github as defined in
	// https://docs.github.com/en/rest/using-the-rest-api/getting-started-with-the-rest-api?apiVersion=2022-11-28#accept
	githubContentTypeJSON = "application/vnd.github+json; charset=utf-8"

	// githubAPIURL is the constant url for contacting Github.
	githubAPIURL = "https://api.github.com"

	// githubPathUserKeys is the Github api path for fetching a users public
	// keys.
	githubPathUserKeys = "users/%s/keys"
)

// PublicKeysForSubject implements the [Resolver] interface by taking a
// Github subject in this case a user and returning all of the public ssh keys
// the user has for their profile.
// The following errors can be expected:
// - [SubjectNotFound] when the subject being asked for does not exist in
// the resolvers domain.
func (g *GithubResolver) PublicKeysForSubject(
	ctx context.Context,
	subject string,
) ([]string, error) {
	url, err := url.Parse(githubAPIURL)
	if err != nil {
		return nil, fmt.Errorf("parsing github url %q: %w", githubAPIURL, err)
	}

	url = url.JoinPath(fmt.Sprintf(githubPathUserKeys, subject))

	req, err := http.NewRequest(http.MethodGet, url.String(), nil)
	if err != nil {
		return nil, fmt.Errorf(
			"generating github user keys request for user %q: %w",
			subject, err,
		)
	}

	req = setAccept(req, githubContentTypeJSON)
	res, err := g.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf(
			"getting github public keys for user %q: %w", subject, err,
		)
	}
	defer res.Body.Close()

	if res.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf(
			"cannot find github user %q%w",
			subject, errors.Hide(SubjectNotFound),
		)
	} else if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(
			"cannot get public keys for github user %q, recieved status %q",
			subject, res.Status,
		)
	}

	safeEncoding, err := hasContentType(res, githubContentTypeJSON)
	if err != nil {
		return nil, fmt.Errorf(
			"cannot determine content type of github public keys response for user %q: %w",
			subject, err,
		)
	}

	if !safeEncoding {
		return nil, fmt.Errorf(
			"github public keys for user %q returned an unknown encoding for response",
			subject,
		)
	}

	ghKeysRes := make([]githubKeyResponse, 0)
	if err := json.NewDecoder(res.Body).Decode(&ghKeysRes); err != nil {
		return nil, fmt.Errorf(
			"decoding github user %q public keys response: %w",
			subject, err,
		)
	}

	rval := make([]string, 0, len(ghKeysRes))
	for _, ghKey := range ghKeysRes {
		rval = append(rval, ghKey.Key)
	}

	return rval, nil
}
