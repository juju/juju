// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/juju/tc"
	gomock "go.uber.org/mock/gomock"

	importererrors "github.com/juju/juju/internal/ssh/importer/errors"
)

type launchpadSuite struct {
	client *MockClient
}

func TestLaunchpadSuite(t *testing.T) {
	tc.Run(t, &launchpadSuite{})
}

func (s *launchpadSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = NewMockClient(ctrl)
	return ctrl
}

// TestSubjectNotFound is asserting that if the [LaunchpadResolver] gets a 404
// return it propagates a [importererrors.SubjectNotFound] error.
func (l *launchpadSuite) TestSubjectNotFound(c *tc.C) {
	defer l.setupMocks(c).Finish()

	l.client.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Check(req.URL.Path, tc.Equals, "/~tlm/+sshkeys")
			c.Check(req.Header.Get("Accept"), tc.Equals, "text/plain;charset=utf-8")
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil
		},
	)

	lp := LaunchpadResolver{l.client}
	_, err := lp.PublicKeysForSubject(c.Context(), "tlm")
	c.Check(err, tc.ErrorIs, importererrors.SubjectNotFound)
}

// TestSubjectPublicKeys is asserting the happy path for the [LaunchpadResolver].
func (l *launchpadSuite) TestSubjectPublicKeys(c *tc.C) {
	defer l.setupMocks(c).Finish()

	l.client.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Check(req.URL.Path, tc.Equals, "/~tlm/+sshkeys")
			c.Check(req.Header.Get("Accept"), tc.Equals, "text/plain;charset=utf-8")

			builder := strings.NewReader(`
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC key1
ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe key2
`)

			return &http.Response{
				Body: io.NopCloser(builder),
				Header: http.Header{
					"Content-Type": []string{contentTypeTextUTF8},
				},
				StatusCode: http.StatusOK,
			}, nil
		},
	)

	lp := LaunchpadResolver{l.client}
	keys, err := lp.PublicKeysForSubject(c.Context(), "tlm")
	c.Check(err, tc.ErrorIsNil)

	expected := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC key1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe key2",
	}

	slices.Sort(keys)
	slices.Sort(expected)

	c.Check(keys, tc.DeepEquals, expected)
}
