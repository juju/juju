package jsval

import (
	"reflect"
	"sync"

	"github.com/lestrrat/go-pdebug"
)

type PropInfo struct {
	// Name of the Field that maps to this property
	FieldName string
	// IsMaybe is true if this property implements the Maybe interface
	IsMaybe bool
}

type StructInfo struct {
	lock  sync.RWMutex
	props map[string]PropInfo
}

type StructInfoRegistry struct {
	lock     sync.RWMutex
	registry map[reflect.Type]StructInfo
}

func (si *StructInfo) FieldName(pname string) (string, bool) {
	si.lock.RLock()
	defer si.lock.RUnlock()

	pinfo, ok := si.props[pname]
	if !ok {
		return "", false
	}
	return pinfo.FieldName, true
}

// Gets the list of property names for this particuar instance of a
// struct. Uninitialized types are not considered, so we remove
// them depending on the state of this instance of the struct
func (si *StructInfo) PropNames(rv reflect.Value) []string {
	si.lock.RLock()
	defer si.lock.RUnlock()

	pnames := make([]string, 0, len(si.props))
	for pname, pinfo := range si.props {
		if pinfo.IsMaybe {
			fv := rv.FieldByName(pinfo.FieldName)
			mv := fv.MethodByName("Valid")
			out := mv.Call(nil)
			if !out[0].Bool() {
				continue
			}
		}

		pnames = append(pnames, pname)
	}
	return pnames
}

func (r *StructInfoRegistry) Lookup(t reflect.Type) (StructInfo, bool) {
	switch t.Kind() {
	case reflect.Ptr, reflect.Interface:
		t = t.Elem()
	}

	r.lock.RLock()
	si, ok := r.registry[t]
	r.lock.RUnlock()

	return si, ok
}

func (r *StructInfoRegistry) Register(t reflect.Type) StructInfo {
	if pdebug.Enabled {
		g := pdebug.Marker("StructInfoRegistry.Register (%s)", t.Name())
		defer g.End()
	}

	switch t.Kind() {
	case reflect.Ptr, reflect.Interface:
		t = t.Elem()
	}

	r.lock.RLock()
	si, ok := r.registry[t]
	r.lock.RUnlock()

	if ok {
		return si
	}

	props := extract(t)
	if pdebug.Enabled {
		pdebug.Printf("Extracted struct with the following properties:")
		for pname, pinfo := range props {
			if pinfo.IsMaybe {
				pdebug.Printf("%s -> %s (Maybe)", pname, pinfo.FieldName)
			} else {
				pdebug.Printf("%s -> %s", pname, pinfo.FieldName)
			}
		}
	}

	si = StructInfo{props: props}
	r.lock.Lock()
	r.registry[t] = si
	r.lock.Unlock()
	return si
}

var maybeif = reflect.TypeOf((*Maybe)(nil)).Elem()

func extract(t reflect.Type) map[string]PropInfo {
	props := make(map[string]PropInfo)
	for i := 0; i < t.NumField(); i++ {
		fv := t.Field(i)
		if fv.Anonymous {
			info := extract(fv.Type)
			for k, v := range info {
				props[k] = v
			}
			continue
		}

		if fv.PkgPath != "" { // not exported
			continue
		}

		// Inspect the JSON struct tag so we know what they want to call
		// this field from JSON.
		tag := fv.Tag.Get("json")
		if tag == "-" { // "Ignore me", says the struct
			continue
		}

		// Inspect the field type. If this field implements the Maybe
		// interface, we want to remember it
		isMaybe := fv.Type.Implements(maybeif) || reflect.PtrTo(fv.Type).Implements(maybeif)
		if pdebug.Enabled {
			pdebug.Printf("Checking if field '%s' implements the Maybe interface -> %t", fv.Name, isMaybe)
		}

		if tag == "" || tag[0] == ',' {
			props[fv.Name] = PropInfo{
				FieldName: fv.Name,
				IsMaybe:   isMaybe,
			}
			continue
		}

		flen := 0
		for j := 0; j < len(tag); j++ {
			if tag[j] == ',' {
				break
			}
			flen = j
		}
		props[tag[:flen+1]] = PropInfo{
			FieldName: fv.Name,
			IsMaybe:   isMaybe,
		}
	}
	return props
}
