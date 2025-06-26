// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package service

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/changestream"
	"github.com/juju/juju/core/machine"
	"github.com/juju/juju/core/machine/testing"
	"github.com/juju/juju/internal/testhelpers"
)

type mapperSuite struct {
	testhelpers.IsolationSuite

	state *MockState
}

func TestMapperSuite(t *stdtesting.T) {
	tc.Run(t, &mapperSuite{})
}

func (s *mapperSuite) TestUuidToNameMapper(c *tc.C) {
	defer s.setupMocks(c).Finish()
	// Arrange
	uuid0 := testing.GenUUID(c).String()
	uuid1 := testing.GenUUID(c).String()

	in := []string{uuid0, uuid1}
	out := map[string]machine.Name{
		uuid0: machine.Name("0"),
		uuid1: machine.Name("1"),
	}
	s.expectGetNamesForUUIDs(in, out)

	changesIn := []changestream.ChangeEvent{
		changeEventShim{
			changeType: 1,
			namespace:  "machine",
			changed:    uuid0,
		},
		changeEventShim{
			changeType: 2,
			namespace:  "machine",
			changed:    uuid1,
		},
	}

	service := s.getService()

	// Act
	changesOut, err := service.uuidToNameMapper(noContainersFilter)(c.Context(), changesIn)

	// Assert
	c.Assert(err, tc.ErrorIsNil)

	c.Check(changesOut, tc.SameContents, []string{"0", "1"})
}

func (s *mapperSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.state = NewMockState(ctrl)
	return ctrl
}

func (s *mapperSuite) getService() *WatchableService {
	return &WatchableService{
		ProviderService: ProviderService{
			Service: Service{st: s.state},
		},
	}
}

func (s *mapperSuite) expectGetNamesForUUIDs(in []string, out map[string]machine.Name) {
	s.state.EXPECT().GetNamesForUUIDs(gomock.Any(), in).Return(out, nil)
}

// changeEventShim implements changestream.ChangeEvent and allows the
// substituting of events in an implementation of eventsource.Mapper.
type changeEventShim struct {
	changeType changestream.ChangeType
	namespace  string
	changed    string
}

// Type returns the type of change (create, update, delete).
func (e changeEventShim) Type() changestream.ChangeType {
	return e.changeType
}

// Namespace returns the namespace of the change. This is normally the
// table name.
func (e changeEventShim) Namespace() string {
	return e.namespace
}

// Changed returns the changed value of event. This logically can be
// the primary key of the row that was changed or the field of the change
// that was changed.
func (e changeEventShim) Changed() string {
	return e.changed
}
