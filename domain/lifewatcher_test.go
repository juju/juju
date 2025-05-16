// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/logger/testing"
)

type lifeWatcherSuite struct {
	dbLifeValues map[string]life.Life
}

type changeEvent struct {
	ctype   changestream.ChangeType
	changed string
}

func (c changeEvent) Type() changestream.ChangeType {
	return c.ctype
}

func (c changeEvent) Namespace() string {
	return "test"
}

func (c changeEvent) Changed() string {
	return c.changed
}
func TestLifeWatcherSuite(t *stdtesting.T) { tc.Run(t, &lifeWatcherSuite{}) }
func (s *lifeWatcherSuite) lifeGetter(ctx context.Context, ids []string) (map[string]life.Life, error) {
	result := make(map[string]life.Life)
	for _, id := range ids {
		if l, ok := s.dbLifeValues[id]; ok {
			result[id] = l
		}
	}
	return result, nil
}

func (s *lifeWatcherSuite) TestInitial(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	})
}

func (s *lifeWatcherSuite) TestChangeNoUpdate(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.HasLen, 0)
}

func (s *lifeWatcherSuite) TestChangeDying(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dying,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	})
}

func (s *lifeWatcherSuite) TestChangeDead(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dead,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	})
}

func (s *lifeWatcherSuite) TestChangNotDeadRemoved(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Delete},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Delete},
	})
}

func (s *lifeWatcherSuite) TestChangeDeadRemoved(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dead,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Delete},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Delete},
	})
}

func (s *lifeWatcherSuite) TestChangeDifferentIdUpdated(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "1", ctype: changestreamtesting.Update},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.HasLen, 0)
}

func (s *lifeWatcherSuite) TestChangeDifferentIdRemoved(c *tc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Create},
	}
	_, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "1", ctype: changestreamtesting.Delete},
	}
	out, err := f(c.Context(), in)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.HasLen, 0)
}
