// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	importererrors "github.com/juju/juju/internal/ssh/importer/errors"
)

type githubSuite struct {
	client *MockClient
}

var (
	_ = tc.Suite(&githubSuite{})
)

func (s *githubSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = NewMockClient(ctrl)
	return ctrl
}

// TestSubjectNotFound is asserting that if the [GithubResolver] gets a 404
// return it propagates a [importererrors.SubjectNotFound] error.
func (g *githubSuite) TestSubjectNotFound(c *tc.C) {
	defer g.setupMocks(c).Finish()

	g.client.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Check(req.URL.Path, tc.Equals, "/users/tlm/keys")
			c.Check(req.Header.Get("Accept"), tc.Equals, "application/json; charset=utf-8")
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil
		},
	)

	gh := GithubResolver{g.client}
	_, err := gh.PublicKeysForSubject(context.Background(), "tlm")
	c.Check(err, tc.ErrorIs, importererrors.SubjectNotFound)
}

// TestSubjectPublicKeys is asserting the happy path for the [GithubResolver].
func (g *githubSuite) TestSubjectPublicKeys(c *tc.C) {
	defer g.setupMocks(c).Finish()

	g.client.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Check(req.URL.Path, tc.Equals, "/users/tlm/keys")
			c.Check(req.Header.Get("Accept"), tc.Equals, "application/json; charset=utf-8")

			res := []githubKeyResponse{
				{
					Id:  1,
					Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
				},
				{
					Id:  2,
					Key: "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
				},
			}

			data, err := json.Marshal(res)
			c.Assert(err, tc.ErrorIsNil)

			return &http.Response{
				Body: io.NopCloser(bytes.NewReader(data)),
				Header: http.Header{
					"Content-Type": []string{githubContentTypeJSON},
				},
				StatusCode: http.StatusOK,
			}, nil
		},
	)

	gh := GithubResolver{g.client}
	keys, err := gh.PublicKeysForSubject(context.Background(), "tlm")
	c.Check(err, tc.ErrorIsNil)

	expected := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC existing1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe existing2",
	}

	slices.Sort(keys)
	slices.Sort(expected)

	c.Check(keys, tc.DeepEquals, expected)
}
