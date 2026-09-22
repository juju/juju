// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"gopkg.in/httprequest.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	backupstesting "github.com/juju/juju/core/backups/testing"
	"github.com/juju/juju/rpc/params"
)

type createSuite struct {
	baseSuite
}

func TestCreateSuite(t *testing.T) {
	tc.Run(t, &createSuite{})
}

func (s *createSuite) TestCreate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	meta := backupstesting.NewMetadata()
	result := params.CreateResult(meta, "juju-backup-test.tar.gz")
	result.Notes = "important"
	resultJSON, err := json.Marshal(result)
	c.Assert(err, tc.ErrorIsNil)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.Assert(r.Method, tc.Equals, http.MethodPost)
		c.Assert(r.URL.String(), tc.Equals, "/backup")

		var args params.BackupsCreateArgs
		c.Assert(json.NewDecoder(r.Body).Decode(&args), tc.ErrorIsNil)
		c.Check(args.Notes, tc.Equals, "important")

		w.Header().Set("Content-Type", params.ContentTypeRaw)
		w.Header().Set(params.BackupMetadataHeader, string(resultJSON))
		_, err := w.Write([]byte("archive data"))
		c.Assert(err, tc.ErrorIsNil)
	}))
	defer srv.Close()
	httpClient := &httprequest.Client{BaseURL: srv.URL}
	s.apiCaller.EXPECT().HTTPClient(base.HTTPClientScopeUnscoped).Return(httpClient, nil)

	client := s.newClient()
	got, rdr, err := client.Create(c.Context(), "important")
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = rdr.Close() }()

	data, err := io.ReadAll(rdr)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(string(data), tc.Equals, "archive data")

	resultMeta := backupstesting.UpdateNotes(meta, "important")
	s.checkMetadataResult(c, &got, resultMeta)
}

// TestCreateError verifies that a classified JSON error from the
// endpoint, as emitted by the apiserver's error sender, reaches the
// caller with its classification intact.
func (s *createSuite) TestCreateError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := json.Marshal(params.ErrorResult{
			Error: &params.Error{
				Code:    params.CodeNotSupported,
				Message: "create-backup from clients older than 4.1 is not supported",
			},
		})
		c.Assert(err, tc.ErrorIsNil)
		w.Header().Set("Content-Type", params.ContentTypeJSON)
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.WriteHeader(http.StatusBadRequest)
		_, err = w.Write(body)
		c.Assert(err, tc.ErrorIsNil)
	}))
	defer srv.Close()
	httpClient := &httprequest.Client{
		BaseURL:        srv.URL,
		UnmarshalError: api.UnmarshalHTTPErrorResponse,
	}
	s.apiCaller.EXPECT().HTTPClient(base.HTTPClientScopeUnscoped).Return(httpClient, nil)

	client := s.newClient()
	_, rdr, err := client.Create(c.Context(), "important")
	c.Assert(err, tc.Satisfies, errors.IsNotSupported)
	c.Assert(rdr, tc.IsNil)
}
