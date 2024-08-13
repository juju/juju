// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/environs"
	environmocks "github.com/juju/juju/environs/testing"
)

// mockSuite is a test suite that uses mocked services and dependencies for the
// spaces API. As we move this facade over to dqlite, we should gradually port
// tests over here.
type mockSuite struct {
	environ *environmocks.MockNetworkingEnviron
}

var _ = gc.Suite(&mockSuite{})

func (s *mockSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.environ = environmocks.NewMockNetworkingEnviron(ctrl)
	return ctrl
}

// TestSupportsSpaces checks that API.checkSupportsSpaces returns nil if spaces
// are supported by the environ.
func (s *mockSuite) TestSupportsSpaces(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.environ.EXPECT().SupportsSpaces(gomock.Any()).Return(true, nil)

	api := &API{
		providerGetter: func(context.Context) (environs.NetworkingEnviron, error) {
			return s.environ, nil
		},
		credentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
	}

	err := api.checkSupportsSpaces(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

// TestSupportsSpacesFalse checks that API.checkSupportsSpaces returns the
// correct error if spaces are not supported by the environ.
func (s *mockSuite) TestSupportsSpacesFalse(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.environ.EXPECT().SupportsSpaces(gomock.Any()).Return(false, nil)

	api := &API{
		providerGetter: func(context.Context) (environs.NetworkingEnviron, error) {
			return s.environ, nil
		},
		credentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
	}

	err := api.checkSupportsSpaces(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

// TestSupportsSpacesError checks that API.checkSupportsSpaces returns the
// correct error if spaces are not supported by the environ.
func (s *mockSuite) TestSupportsSpacesError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.environ.EXPECT().SupportsSpaces(gomock.Any()).Return(false, errors.NotSupportedf("spaces"))

	api := &API{
		providerGetter: func(context.Context) (environs.NetworkingEnviron, error) {
			return s.environ, nil
		},
		credentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
	}

	err := api.checkSupportsSpaces(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}

// TestSupportsSpacesError checks that API.checkSupportsSpaces returns the
// correct error if the environ does not support networking.
func (s *mockSuite) TestSupportsSpacesNetworkingNotSupported(c *gc.C) {
	defer s.setupMocks(c).Finish()

	api := &API{
		providerGetter: func(context.Context) (environs.NetworkingEnviron, error) {
			return nil, errors.NotSupportedf("networking")
		},
		credentialInvalidatorGetter: apiservertesting.NoopModelCredentialInvalidatorGetter,
	}

	err := api.checkSupportsSpaces(context.Background())
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}
