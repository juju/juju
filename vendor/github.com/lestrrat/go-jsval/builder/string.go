package builder

import (
	"errors"

	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
)

func buildStringConstraint(ctx *buildctx, c *jsval.StringConstraint, s *schema.Schema) error {
	if len(s.Type) > 0 {
		if !s.Type.Contains(schema.StringType) {
			return errors.New("schema is not for string")
		}
	}

	if s.MaxLength.Initialized {
		c.MaxLength(s.MaxLength.Val)
	}

	if s.MinLength.Initialized {
		c.MinLength(s.MinLength.Val)
	}

	if pat := s.Pattern; pat != nil {
		c.Regexp(pat)
	}

	if f := s.Format; f != "" {
		c.Format(string(f))
	}

	if lst := s.Enum; len(lst) > 0 {
		c.Enum(lst...)
	}

	if v := s.Default; v != nil {
		c.Default(v)
	}

	return nil
}
