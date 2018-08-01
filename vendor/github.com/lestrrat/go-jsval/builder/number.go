package builder

import (
	"errors"

	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
)

func buildIntegerConstraint(ctx *buildctx, nc *jsval.IntegerConstraint, s *schema.Schema) error {
	if len(s.Type) > 0 {
		if !s.Type.Contains(schema.IntegerType) {
			return errors.New("schema is not for integer")
		}
	}

	if s.Minimum.Initialized {
		nc.Minimum(s.Minimum.Val)
		if s.ExclusiveMinimum.Initialized {
			nc.ExclusiveMinimum(s.ExclusiveMinimum.Val)
		}
	}

	if s.Maximum.Initialized {
		nc.Maximum(s.Maximum.Val)
		if s.ExclusiveMaximum.Initialized {
			nc.ExclusiveMaximum(s.ExclusiveMaximum.Val)
		}
	}

	if s.MultipleOf.Initialized {
		nc.MultipleOf(s.MultipleOf.Val)
	}

	if lst := s.Enum; len(lst) > 0 {
		nc.Enum(lst...)
	}

	if v := s.Default; v != nil {
		nc.Default(v)
	}

	return nil
}

func buildNumberConstraint(ctx *buildctx, nc *jsval.NumberConstraint, s *schema.Schema) error {
	if len(s.Type) > 0 {
		if !s.Type.Contains(schema.NumberType) {
			return errors.New("schema is not for number")
		}
	}

	if s.Minimum.Initialized {
		nc.Minimum(s.Minimum.Val)
		if s.ExclusiveMinimum.Initialized {
			nc.ExclusiveMinimum(s.ExclusiveMinimum.Val)
		}
	}

	if s.Maximum.Initialized {
		nc.Maximum(s.Maximum.Val)
		if s.ExclusiveMaximum.Initialized {
			nc.ExclusiveMaximum(s.ExclusiveMaximum.Val)
		}
	}

	if s.MultipleOf.Initialized {
		nc.MultipleOf(s.MultipleOf.Val)
	}

	if lst := s.Enum; len(lst) > 0 {
		nc.Enum(lst)
	}

	if v := s.Default; v != nil {
		nc.Default(v)
	}

	return nil
}