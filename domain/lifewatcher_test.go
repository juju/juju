// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/changestream"
	changestreamtesting "github.com/juju/juju/core/changestream/testing"
	coredatabase "github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/life"
	"github.com/juju/juju/internal/logger/testing"
)

type lifeWatcherSuite struct {
	dbLifeValues map[string]life.Life
	db           coredatabase.TxnRunner
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

var _ = tc.Suite(&lifeWatcherSuite{})

func (s *lifeWatcherSuite) lifeGetter(ctx context.Context, db coredatabase.TxnRunner, ids []string) (map[string]life.Life, error) {
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
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dying,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dead,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Delete},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dead,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestreamtesting.Delete},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "1", ctype: changestreamtesting.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
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
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "1", ctype: changestreamtesting.Delete},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, tc.HasLen, 0)
}
