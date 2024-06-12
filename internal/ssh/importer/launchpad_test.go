// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package importer

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"

	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type launchpadSuite struct {
	client *MockClient
}

var (
	_ = gc.Suite(&githubSuite{})
)

func (s *launchpadSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.client = NewMockClient(ctrl)
	return ctrl
}

// TestSubjectNotFound is asserting that if the [LaunchpadResolver] gets a 404
// return it propogates a [SubjectNotFound] error.
func (l *launchpadSuite) TestSubjectNotFound(c *gc.C) {
	defer l.setupMocks(c).Finish()

	l.client.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Check(req.URL.Path, gc.Equals, "/~tlm/+sshkeys")
			c.Check(req.Header.Get("Accept"), gc.Equals, "text/plain; charset=utf-8")
			return &http.Response{
				Body:       io.NopCloser(strings.NewReader("")),
				StatusCode: http.StatusNotFound,
			}, nil
		},
	)

	lp := LaunchpadResolver{l.client}
	_, err := lp.PublicKeysForSubject(context.Background(), "~tlm")
	c.Check(err, jc.ErrorIs, SubjectNotFound)
}

// TestSubjectPublicKeys is asserting the happy path for the [LaunchpadResolver].
func (l *launchpadSuite) TestSubjectPublicKeys(c *gc.C) {
	defer l.setupMocks(c).Finish()

	l.client.EXPECT().Do(gomock.Any()).DoAndReturn(
		func(req *http.Request) (*http.Response, error) {
			c.Check(req.URL.Path, gc.Equals, "/~tlm/+sshkeys")
			c.Check(req.Header.Get("Accept"), gc.Equals, "text/plain; charset=utf-8")

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
	keys, err := lp.PublicKeysForSubject(context.Background(), "~tlm")
	c.Check(err, jc.ErrorIsNil)

	expected := []string{
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAII4GpCvqUUYUJlx6d1kpUO9k/t4VhSYsf0yE0/QTqDzC key1",
		"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJQJ9wv0uC3yytXM3d2sJJWvZLuISKo7ZHwafHVviwVe key2",
	}

	slices.Sort(keys)
	slices.Sort(expected)

	c.Check(keys, jc.DeepEquals, expected)
}
