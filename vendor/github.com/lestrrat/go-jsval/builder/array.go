package builder

import (
	"github.com/lestrrat/go-jsschema"
	"github.com/lestrrat/go-jsval"
	"github.com/lestrrat/go-pdebug"
)

func buildArrayConstraint(ctx *buildctx, c *jsval.ArrayConstraint, s *schema.Schema) (err error) {
	if pdebug.Enabled {
		g := pdebug.IPrintf("START buildArrayConstraint")
		defer func() {
			if err == nil {
				g.IRelease("END buildArrayConstraint (PASS)")
			} else {
				g.IRelease("END buildArrayConstraint (FAIL): %s", err)
			}
		}()
	}

	if items := s.Items; items != nil {
		if !items.TupleMode {
			specs, err := buildFromSchema(ctx, items.Schemas[0])
			if err != nil {
				return err
			}
			c.Items(specs)
		} else {
			specs := make([]jsval.Constraint, len(items.Schemas))
			for i, espec := range items.Schemas {
				item, err := buildFromSchema(ctx, espec)
				if err != nil {
					return err
				}
				specs[i] = item
			}
			c.PositionalItems(specs)

			aitems := s.AdditionalItems
			if aitems == nil {
				if pdebug.Enabled {
					pdebug.Printf("Disabling additional items")
				}
				// No additional items
				c.AdditionalItems(nil)
			} else if as := aitems.Schema; as != nil {
				spec, err := buildFromSchema(ctx, as)
				if err != nil {
					return err
				}
				if pdebug.Enabled {
					pdebug.Printf("Using constraint for additional items ")
				}
				c.AdditionalItems(spec)
			} else {
				if pdebug.Enabled {
					pdebug.Printf("Additional items will be allowed freely")
				}
				c.AdditionalItems(jsval.EmptyConstraint)
			}
		}
	}

	if s.MinItems.Initialized {
		c.MinItems(s.MinItems.Val)
	}

	if s.MaxItems.Initialized {
		c.MaxItems(s.MaxItems.Val)
	}

	if s.UniqueItems.Initialized {
		c.UniqueItems(s.UniqueItems.Val)
	}

	return nil
}