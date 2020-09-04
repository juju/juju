// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charmhub

import (
	"crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/juju/charm/v8"
	"github.com/juju/errors"
)

const (
	// ContentHashHeader specifies the header attribute
	// that will hold the content hash for archive GET responses.
	ContentHashHeader = "Content-Sha384"

	// EntityIdHeader specifies the header attribute that will hold the
	// id of the entity for archive GET responses.
	EntityIdHeader = "Entity-Id"
)

type GetClient struct {
	client RESTClient
}

// NewGetClient creates a GetClient for requesting
func NewGetClient(client RESTClient) *GetClient {
	return &GetClient{
		client: client,
	}
}

// GetCharmFromURL returns a charm archive retrieved from the given
// URL.
func (c *GetClient) GetCharmFromURL(curl *url.URL, archivePath string) (*charm.CharmArchive, error) {
	f, err := os.Create(archivePath)
	if err != nil {
		return nil, errors.Trace(err)
	}
	defer func() { _ = f.Close() }()
	if err := c.copyArchive(curl, f); err != nil {
		return nil, errors.Trace(err)
	}
	return charm.ReadCharmArchive(archivePath)
}

// copyArchive reads the archive from the given charm or bundle URL
// and writes it to the given writer.\
func (c *GetClient) copyArchive(curl *url.URL, w io.Writer) error {
	r, _, expectHash, expectSize, err := c.getArchive(curl)
	if err != nil {
		return errors.Annotatef(err, "cannot retrieve %q", curl)
	}
	defer func() { _ = r.Close() }()

	hash := sha512.New384()
	size, err := io.Copy(io.MultiWriter(hash, w), r)
	if err != nil {
		return errors.Annotatef(err, "cannot read entity archive")
	}
	if size != expectSize {
		return errors.Errorf("size mismatch; network corruption?")
	}
	if fmt.Sprintf("%x", hash.Sum(nil)) != expectHash {
		return errors.Errorf("hash mismatch; network corruption?")
	}
	return nil
}

func (c *GetClient) getArchive(curl *url.URL) (r io.ReadCloser, eid *charm.URL, hash string, size int64, err error) {
	fail := func(err error) (io.ReadCloser, *charm.URL, string, int64, error) {
		return nil, nil, "", 0, err
	}
	// Create the request.
	req, err := http.NewRequest("GET", curl.String(), nil)
	if err != nil {
		return fail(errors.Annotatef(err, "cannot make new request"))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		//terr := params.MaybeTermsAgreementError(err)
		//if err1, ok := errors.Cause(terr).(*params.TermAgreementRequiredError); ok {
		//	terms := strings.Join(err1.Terms, " ")
		//	return fail(errors.Errorf(`cannot get archive because some terms have not been agreed to. Try "juju agree %s"`, terms))
		//}
		return fail(errors.Annotate(err, "cannot get archive"))
	}

	// Validate the response headers.
	entityId := resp.Header.Get(EntityIdHeader)
	if entityId == "" {
		defer func() { _ = resp.Body.Close() }()
		return fail(errors.Errorf("no %s header found in response", EntityIdHeader))
	}
	eid, err = charm.ParseURL(entityId)
	if err != nil {
		// The server did not return a valid id.
		defer func() { _ = resp.Body.Close() }()
		return fail(errors.Annotatef(err, "invalid entity id found in response"))
	}
	if eid.Revision == -1 {
		// The server did not return a fully qualified entity id.
		defer func() { _ = resp.Body.Close() }()
		return fail(errors.Errorf("archive get returned not fully qualified entity id %q", eid))
	}
	hash = resp.Header.Get(ContentHashHeader)
	if hash == "" {
		defer func() { _ = resp.Body.Close() }()
		return fail(errors.Errorf("no %s header found in response", ContentHashHeader))
	}

	// Validate the response contents.
	if resp.ContentLength < 0 {
		// TODO frankban: handle the case the contents are chunked.
		defer func() { _ = resp.Body.Close() }()
		return fail(errors.Errorf("no content length found in response"))
	}
	return resp.Body, eid, hash, resp.ContentLength, nil
}
