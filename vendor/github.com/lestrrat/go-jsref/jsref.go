package jsref

import (
	"net/url"
	"reflect"

	"github.com/lestrrat/go-jspointer"
	"github.com/lestrrat/go-pdebug"
	"github.com/lestrrat/go-structinfo"
	"github.com/pkg/errors"
)

var DefaultMaxRecursions = 10

// New creates a new Resolver
func New() *Resolver {
	return &Resolver{MaxRecursions: DefaultMaxRecursions}
}

// AddProvider adds a new Provider to be searched for in case
// a JSON pointer with more than just the URI fragment is given.
func (r *Resolver) AddProvider(p Provider) error {
	r.providers = append(r.providers, p)
	return nil
}

type resolveCtx struct {
	rlevel    int         // recurse level
	maxrlevel int         // max recurse level
	object    interface{} // the main object that was passed to `Resolve()`
}

// Resolve takes a target `v`, and a JSON pointer `spec`.
// spec is expected to be in the form of
//
//    [scheme://[userinfo@]host/path[?query]]#fragment
//    [scheme:opaque[?query]]#fragment
//
// where everything except for `#fragment` is optional.
//
// If `spec` is the empty string, `v` is returned
// This method handles recursive JSON references.
func (r *Resolver) Resolve(v interface{}, ptr string) (ret interface{}, err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("Resolver.Resolve(%s)", ptr).BindError(&err)
		defer g.End()
	}

	ctx := resolveCtx{
		rlevel:    0,
		maxrlevel: r.MaxRecursions,
		object:    v,
	}

	// First, expand the target as much as we can
	v, err = expandRefRecursive(ctx, r, v)
	if err != nil {
		return nil, errors.Wrap(err, "recursive search failed")
	}

	return evalptr(ctx, r, v, ptr)
}

func expandRefRecursive(ctx resolveCtx, r *Resolver, v interface{}) (ret interface{}, err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("expandRefRecursive")
		defer g.End()
	}
	for {
		ref, err := findRef(v)
		if err != nil {
			if pdebug.Enabled {
				pdebug.Printf("No refs found. bailing out of loop")
			}
			break
		}

		if pdebug.Enabled {
			pdebug.Printf("Found ref '%s'", ref)
		}

		newv, err := expandRef(ctx, r, v, ref)
		if err != nil {
			if pdebug.Enabled {
				pdebug.Printf("Failed to expand ref '%s': %s", ref, err)
			}
			return nil, errors.Wrap(err, "failed to expand ref")
		}

		v = newv
	}

	return v, nil
}

func expandRef(ctx resolveCtx, r *Resolver, v interface{}, ref string) (ret interface{}, err error) {
	ctx.rlevel++
	if ctx.rlevel > ctx.maxrlevel {
		return nil, ErrMaxRecursion
	}

	defer func() { ctx.rlevel-- }()

	u, err := url.Parse(ref)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ref as URL")
	}

	ptr := "#" + u.Fragment
	if u.Host == "" && u.Path == "" {
		if pdebug.Enabled {
			pdebug.Printf("ptr doesn't contain any host/path part, apply json pointer directly to object")
		}
		return evalptr(ctx, r, ctx.object, ptr)
	}

	u.Fragment = ""
	for _, p := range r.providers {
		pv, err := p.Get(u)
		if err == nil {
			if pdebug.Enabled {
				pdebug.Printf("Found object matching %s", u)
			}

			return evalptr(ctx, r, pv, ptr)
		}
	}

	return nil, errors.New("element pointed by $ref '" + ref + "' not found")
}

func findRef(v interface{}) (ref string, err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("findRef").BindError(&err)
		defer g.End()
	}

	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Interface, reflect.Ptr:
		rv = rv.Elem()
	}

	if pdebug.Enabled {
		pdebug.Printf("object is a '%s'", rv.Kind())
	}

	// Find if we have a "$ref" element
	refv := zeroval
	switch rv.Kind() {
	case reflect.Map:
		refv = rv.MapIndex(reflect.ValueOf("$ref"))
	case reflect.Struct:
		if fn := structinfo.StructFieldFromJSONName(rv, "$ref"); fn != "" {
			refv = rv.FieldByName(fn)
		}
	default:
		return "", errors.New("element is not a map-like container")
	}

	switch refv.Kind() {
	case reflect.Interface, reflect.Ptr:
		refv = refv.Elem()
	}

	switch refv.Kind() {
	case reflect.String:
		// Empty string isn't a valid pointer
		ref := refv.String()
		if ref == "" {
			return "", errors.New("$ref element not found (empty)")
		}
		if pdebug.Enabled {
			pdebug.Printf("Found ref '%s'", ref)
		}
		return ref, nil
	case reflect.Invalid:
		return "", errors.New("$ref element not found")
	default:
		if pdebug.Enabled {
			pdebug.Printf("'$ref' was found, but its kind is %s", refv.Kind())
		}
	}

	return "", errors.New("$ref element must be a string")
}

func evalptr(ctx resolveCtx, r *Resolver, v interface{}, ptrspec string) (ret interface{}, err error) {
	if pdebug.Enabled {
		g := pdebug.Marker("evalptr(%s)", ptrspec).BindError(&err)
		defer g.End()
	}

	// If the reference is empty, return v
	if ptrspec == "" || ptrspec == "#" {
		if pdebug.Enabled {
			pdebug.Printf("Empty pointer, return v itself")
		}
		return v, nil
	}

	// Parse the spec.
	u, err := url.Parse(ptrspec)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse reference spec")
	}

	ptr := u.Fragment
	p, err := jspointer.New(ptr)
	if err != nil {
		return nil, errors.Wrap(err, "failed create a new JSON pointer")
	}
	x, err := p.Get(v)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch value")
	}

	if pdebug.Enabled {
		pdebug.Printf("Evaulated JSON pointer, now checking if we can expand further")
	}
	// If this result contains more refs, expand that
	return expandRefRecursive(ctx, r, x)
}
