// Copyright 2033 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package domain

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/changestream"
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

var _ = gc.Suite(&lifeWatcherSuite{})

func (s *lifeWatcherSuite) lifeGetter(ctx context.Context, db coredatabase.TxnRunner, ids ...string) (map[string]life.Life, error) {
	result := make(map[string]life.Life)
	for _, id := range ids {
		if l, ok := s.dbLifeValues[id]; ok {
			result[id] = l
		}
	}
	return result, nil
}

func (s *lifeWatcherSuite) TestInitial(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	})
}

func (s *lifeWatcherSuite) TestChangeNoUpdate(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.HasLen, 0)
}

func (s *lifeWatcherSuite) TestChangeDying(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dying,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Update},
	})
}

func (s *lifeWatcherSuite) TestChangeDead(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dead,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Update},
	})
}

func (s *lifeWatcherSuite) TestChangNotDeadRemoved(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Delete},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Delete},
	})
}

func (s *lifeWatcherSuite) TestChangeDeadRemoved(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	s.dbLifeValues = map[string]life.Life{
		"0": life.Dead,
	}
	in = []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Delete},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, jc.DeepEquals, []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Delete},
	})
}

func (s *lifeWatcherSuite) TestChangeDifferentIdUpdated(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "1", ctype: changestream.Update},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.HasLen, 0)
}

func (s *lifeWatcherSuite) TestChangeDifferentIdRemoved(c *gc.C) {
	s.dbLifeValues = map[string]life.Life{
		"0": life.Alive,
	}
	// Initial event.
	f := LifeStringsWatcherMapperFunc(testing.WrapCheckLog(c), s.lifeGetter)
	in := []changestream.ChangeEvent{
		changeEvent{changed: "0", ctype: changestream.Create},
	}
	_, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)

	in = []changestream.ChangeEvent{
		changeEvent{changed: "1", ctype: changestream.Delete},
	}
	out, err := f(context.Background(), s.db, in)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.HasLen, 0)
}
