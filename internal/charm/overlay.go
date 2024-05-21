// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"encoding/base64"
	"fmt"
	"math"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/juju/errors"
	"github.com/mohae/deepcopy"
)

// ExtractBaseAndOverlayParts splits the bundle data into a base and
// overlay-specific bundle so that their union yields bd. To decide whether a
// field is overlay-specific, the implementation uses reflection and
// recursively scans the BundleData fields looking for fields annotated with
// the "overlay-only: true" tag.
//
// To produce the base bundle, the original bundle is filtered and all
// overlay-specific values are set to the zero value for their type. To produce
// the overlay-specific bundle, we once again filter the original bundle but
// this time zero out fields that do not contain any descendant fields that are
// overlay-specific.
//
// To clarify how this method works let's consider a bundle created via the
// yaml blob below:
//
//   applications:
//     apache2:
//       charm: cs:apache2-26
//       offers:
//         my-offer:
//           endpoints:
//           - apache-website
//           - website-cache
//         my-other-offer:
//           endpoints:
//           - apache-website
//   series: bionic
//
// The "offers" and "endpoints" attributes are overlay-specific fields. If we
// were to run this method and then marshal the results back to yaml we would
// get:
//
// The base bundle:
//
//   applications:
//     apache2:
//       charm: cs:apache2-26
//   series: bionic
//
// The overlay-specific bundle:
//
//   applications:
//     apache2:
//       offers:
//         my-offer:
//           endpoints:
//           - apache-website
//           - website-cache
//         my-other-offer:
//           endpoints:
//           - apache-website
//
// The two bundles returned by this method are copies of the original bundle
// data and can thus be safely manipulated by the caller.
func ExtractBaseAndOverlayParts(bd *BundleData) (base, overlay *BundleData, err error) {
	base = cloneBundleData(bd)
	_ = visitField(&visitorContext{
		structVisitor:          clearOverlayFields,
		dropNonRequiredMapKeys: false,
	}, base)

	overlay = cloneBundleData(bd)
	_ = visitField(&visitorContext{
		structVisitor:          clearNonOverlayFields,
		dropNonRequiredMapKeys: true,
	}, overlay)

	return base, overlay, nil
}

// cloneBundleData uses the gob package to perform a deep copy of bd.
func cloneBundleData(bd *BundleData) *BundleData {
	return deepcopy.Copy(bd).(*BundleData)
}

// VerifyNoOverlayFieldsPresent scans the contents of bd and returns an error
// if the bundle contains any overlay-specific values.
func VerifyNoOverlayFieldsPresent(bd *BundleData) error {
	var (
		errList   []error
		pathStack []string
	)

	ctx := &visitorContext{
		structVisitor: func(ctx *visitorContext, val reflect.Value, typ reflect.Type) (foundOverlay bool) {
			for i := 0; i < typ.NumField(); i++ {
				structField := typ.Field(i)

				// Skip non-exportable and empty fields
				v := val.Field(i)
				if !v.CanInterface() || isZero(v) {
					continue
				}

				if isOverlayField(structField) {
					errList = append(
						errList,
						fmt.Errorf(
							"%s.%s can only appear in an overlay section",
							strings.Join(pathStack, "."),
							yamlName(structField),
						),
					)
					foundOverlay = true
				}

				pathStack = append(pathStack, yamlName(structField))
				if visitField(ctx, v.Interface()) {
					foundOverlay = true
				}
				pathStack = pathStack[:len(pathStack)-1]
			}
			return foundOverlay
		},
		indexedElemPreVisitor: func(index interface{}) {
			pathStack = append(pathStack, fmt.Sprint(index))
		},
		indexedElemPostVisitor: func(_ interface{}) {
			pathStack = pathStack[:len(pathStack)-1]
		},
	}

	_ = visitField(ctx, bd)
	if len(errList) == 0 {
		return nil
	}

	return &VerificationError{errList}
}

func yamlName(structField reflect.StructField) string {
	fields := strings.Split(structField.Tag.Get("yaml"), ",")
	if len(fields) == 0 || fields[0] == "" {
		return strings.ToLower(structField.Name)
	}

	return fields[0]
}

type visitorContext struct {
	structVisitor func(ctx *visitorContext, val reflect.Value, typ reflect.Type) bool

	// An optional pre/post visitor for indexable items (slices, maps)
	indexedElemPreVisitor  func(index interface{})
	indexedElemPostVisitor func(index interface{})

	dropNonRequiredMapKeys bool
}

// visitField invokes ctx.structVisitor(val) if v is a struct and returns back
// the visitor's result. On the other hand, if val is a slice or a map,
// visitField invoke specialized functions that support iterating such types.
func visitField(ctx *visitorContext, val interface{}) bool {
	if val == nil {
		return false
	}
	typ := reflect.TypeOf(val)
	v := reflect.ValueOf(val)

	// De-reference pointers
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
		if v.Kind() == reflect.Invalid {
			return false
		}
		typ = v.Type()
	}

	switch typ.Kind() {
	case reflect.Struct:
		return ctx.structVisitor(ctx, v, typ)
	case reflect.Map:
		return visitFieldsInMap(ctx, v)
	case reflect.Slice:
		return visitFieldsInSlice(ctx, v)
	}

	// v is not a struct or something we can iterate to reach a struct
	return false
}

// visitFieldsInMap iterates the map specified by val and recursively visits
// each map element. The returned value is the logical OR of the responses
// returned by visiting all map elements.
func visitFieldsInMap(ctx *visitorContext, val reflect.Value) (result bool) {
	for _, key := range val.MapKeys() {
		v := val.MapIndex(key)
		if !v.CanInterface() {
			continue
		}

		if ctx.indexedElemPreVisitor != nil {
			ctx.indexedElemPreVisitor(key)
		}

		visRes := visitField(ctx, v.Interface())
		result = visRes || result

		// If the map value is a non-scalar value and the visitor
		// returned false (don't retain), consult the dropNonRequiredMapKeys
		// hint to decide whether we need to delete the key from the map.
		//
		// This is required when splitting bundles into base/overlay
		// bits as empty map values would be encoded as empty objects
		// that the overlay merge code would mis-interpret as deletions.
		if !visRes && isNonScalar(v) && ctx.dropNonRequiredMapKeys {
			val.SetMapIndex(key, reflect.Value{})
		}

		if ctx.indexedElemPostVisitor != nil {
			ctx.indexedElemPostVisitor(key)
		}
	}

	return result
}

// visitFieldsInSlice iterates the slice specified by val and recursively
// visits each element. The returned value is the logical OR of the responses
// returned by visiting all slice elements.
func visitFieldsInSlice(ctx *visitorContext, val reflect.Value) (result bool) {
	for i := 0; i < val.Len(); i++ {
		v := val.Index(i)
		if !v.CanInterface() {
			continue
		}

		if ctx.indexedElemPreVisitor != nil {
			ctx.indexedElemPreVisitor(i)
		}

		result = visitField(ctx, v.Interface()) || result

		if ctx.indexedElemPostVisitor != nil {
			ctx.indexedElemPostVisitor(i)
		}
	}

	return result
}

// clearOverlayFields is an implementation of structVisitor. It recursively
// visits all fields in the val struct and sets the ones that are tagged as
// overlay-only to the zero value for their particular type.
func clearOverlayFields(ctx *visitorContext, val reflect.Value, typ reflect.Type) (retainAncestors bool) {
	for i := 0; i < typ.NumField(); i++ {
		structField := typ.Field(i)

		// Skip non-exportable and empty fields
		v := val.Field(i)
		if !v.CanInterface() || isZero(v) {
			continue
		}

		// No need to recurse further down; just erase the field
		if isOverlayField(structField) {
			v.Set(reflect.Zero(v.Type()))
			continue
		}

		_ = visitField(ctx, v.Interface())
		retainAncestors = true
	}
	return retainAncestors
}

// clearNonOverlayFields is an implementation of structVisitor. It recursively
// visits all fields in the val struct and sets any field that does not contain
// any overlay-only descendants to the zero value for its particular type.
func clearNonOverlayFields(ctx *visitorContext, val reflect.Value, typ reflect.Type) (retainAncestors bool) {
	for i := 0; i < typ.NumField(); i++ {
		structField := typ.Field(i)

		// Skip non-exportable and empty fields
		v := val.Field(i)
		if !v.CanInterface() || isZero(v) {
			continue
		}

		// If this is an overlay field we need to preserve it and all
		// its ancestor fields up to the root. However, we still need
		// to visit its descendants in case we need to clear additional
		// non-overlay fields further down the tree.
		isOverlayField := isOverlayField(structField)
		if isOverlayField {
			retainAncestors = true
		}

		target := v.Interface()
		if retain := visitField(ctx, target); !isOverlayField && !retain {
			v.Set(reflect.Zero(v.Type()))
			continue
		}

		retainAncestors = true
	}
	return retainAncestors
}

// isOverlayField returns true if a struct field is tagged as overlay-only.
func isOverlayField(structField reflect.StructField) bool {
	return structField.Tag.Get("source") == "overlay-only"
}

// isZero reports whether v is the zero value for its type. It panics if the
// argument is invalid. The implementation has been copied from the upstream Go
// repo as it has not made its way to a stable Go release yet.
func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.Invalid:
		return true
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return math.Float64bits(v.Float()) == 0
	case reflect.Complex64, reflect.Complex128:
		c := v.Complex()
		return math.Float64bits(real(c)) == 0 && math.Float64bits(imag(c)) == 0
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if !isZero(v.Index(i)) {
				return false
			}
		}
		return true
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice, reflect.UnsafePointer:
		return v.IsNil()
	case reflect.String:
		return v.Len() == 0
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if !isZero(v.Field(i)) {
				return false
			}
		}
		return true
	default:
		// This should never happens, but will act as a safeguard for
		// later, as a default value doesn't makes sense here.
		panic(fmt.Sprintf("unexpected value of type %s passed to isZero", v.Kind().String()))
	}
}

// ReadAndMergeBundleData reads N bundle data sources, composes their contents
// together and returns the result. The first bundle data source is treated as
// a base bundle while subsequent bundle data sources are treated as overlays
// which are sequentially merged onto the base bundle.
//
// Before returning the merged bundle, ReadAndMergeBundleData will also attempt
// to resolve any include directives present in the machine annotations,
// application options and annotations.
//
// When merging an overlay into a base bundle the following rules apply for the
// BundleData struct fields:
// - if an overlay specifies a bundle-level series, it overrides the base bundle
//   series.
// - overlay-defined relations are appended to the base bundle relations
// - overlay-defined machines overwrite the base bundle machines.
// - if an overlay defines an application that is not present in the base bundle,
//   it will get appended to the application list.
// - if an overlay defines an empty application or saas value, it will be removed
//   from the base bundle together with any associated relations. For example, to
//   remove an application named "mysql" the following overlay snippet can be
//   provided:
//     applications:
//       mysql:
//
// - if an overlay defines an application that is also present in the base bundle
//   the two application specs are merged together (see following rules)
//
// ApplicationSpec merge rules:
// - if the overlay defines a value for a scalar or slice field, it will overwrite
//   the value from the base spec (e.g. trust, series etc).
// - if the overlay specifies a nil/empty value for a map field, then the map
//   field of the base spec will be cleared.
// - if the overlay specifies a non-empty value for a map field, its key/value
//   tuples are iterated and:
//   - if the value is nil/zero and the value is non-scalar, it is deleted from
//     the base spec.
//   - otherwise, the key/value is inserted into the base spec overwriting any
//     existing entries.
func ReadAndMergeBundleData(sources ...BundleDataSource) (*BundleData, error) {
	var allParts []*BundleDataPart
	var partSrcIndex []int
	for srcIndex, src := range sources {
		if src == nil {
			continue
		}

		for _, part := range src.Parts() {
			allParts = append(allParts, part)
			partSrcIndex = append(partSrcIndex, srcIndex)
		}
	}

	if len(allParts) == 0 {
		return nil, errors.NotValidf("malformed bundle: bundle is empty")
	}

	// Treat the first part as the base bundle
	base := allParts[0]
	if err := VerifyNoOverlayFieldsPresent(base.Data); err != nil {
		return nil, errors.Trace(err)
	}

	// Merge parts and resolve include directives
	for index, part := range allParts {
		// Resolve any re-writing of normalisation that could cause the presence
		// field to be out of sync with the actual bundle representation.
		resolveOverlayPresenceFields(part)

		if index != 0 {
			if err := applyOverlay(base, part); err != nil {
				return nil, errors.Trace(err)
			}
		}

		// Relative include directives are resolved using the base path
		// of the datasource that yielded this part
		srcIndex := partSrcIndex[index]
		incResolver := sources[srcIndex].ResolveInclude
		basePath := sources[srcIndex].BasePath()
		for app, appData := range base.Data.Applications {
			if appData == nil {
				return nil, errors.Errorf("base application %q has no body", app)
			}
			resolvedCharm, err := resolveRelativeCharmPath(basePath, appData.Charm)
			if err != nil {
				return nil, errors.Annotatef(err, "resolving relative charm path %q for application %q", appData.Charm, app)
			}
			appData.Charm = resolvedCharm

			for k, v := range appData.Options {
				newV, changed, err := resolveIncludes(incResolver, v)
				if err != nil {
					return nil, errors.Annotatef(err, "processing option %q for application %q", k, app)
				}
				if changed {
					appData.Options[k] = newV
				}
			}

			for k, v := range appData.Annotations {
				newV, changed, err := resolveIncludes(incResolver, v)
				if err != nil {
					return nil, errors.Annotatef(err, "processing annotation %q for application %q", k, app)
				}
				if changed {
					appData.Annotations[k] = newV
				}
			}
		}

		for machine, machineData := range base.Data.Machines {
			if machineData == nil {
				continue
			}

			for k, v := range machineData.Annotations {
				newV, changed, err := resolveIncludes(incResolver, v)
				if err != nil {
					return nil, errors.Annotatef(err, "processing annotation %q for machine %q", k, machine)
				}
				if changed {
					machineData.Annotations[k] = newV
				}
			}
		}
	}

	return base.Data, nil
}

// resolveOverlayPresenceFields exists because we expose an internal bundle
// representation of a type out to the consumers of the library. This means it
// becomes very difficult to know what was re-written during the normalisation
// phase, without telling downstream consumers.
//
// The following attempts to guess when a normalisation has occurred, but the
// presence field map is out of sync with the new changes.
func resolveOverlayPresenceFields(base *BundleDataPart) {
	applications := base.PresenceMap.forField("applications")
	if len(applications) == 0 {
		return
	}
	for name, app := range base.Data.Applications {
		if !applications.fieldPresent(name) {
			continue
		}

		presence := applications.forField(name)
		// If the presence map contains scale, but doesn't contain num_units
		// and if the app.Scale_ has been set to zero. We can then assume that a
		// normalistion has occurred.
		if presence.fieldPresent("scale") && !presence.fieldPresent("num_units") && app.Scale_ == 0 && app.NumUnits > 0 {
			presence["num_units"] = presence["scale"]
		}
	}
}

func applyOverlay(base, overlay *BundleDataPart) error {
	if overlay == nil || len(overlay.PresenceMap) == 0 {
		return nil
	}
	if !overlay.PresenceMap.fieldPresent("applications") && len(overlay.Data.Applications) > 0 {
		return errors.Errorf("bundle overlay file used deprecated 'services' key, this is not valid for bundle overlay files")
	}

	// Merge applications
	if len(overlay.Data.Applications) != 0 {
		if base.Data.Applications == nil {
			base.Data.Applications = make(map[string]*ApplicationSpec, len(overlay.Data.Applications))
		}

		fpm := overlay.PresenceMap.forField("applications")
		for srcAppName, srcAppSpec := range overlay.Data.Applications {
			// If the overlay map points to an empty object, delete
			// it from the base bundle
			if isZero(reflect.ValueOf(srcAppSpec)) {
				delete(base.Data.Applications, srcAppName)
				base.Data.Relations = removeRelations(base.Data.Relations, srcAppName)
				continue
			}

			// If this is a new application just append it; otherwise
			// recursively merge the two application specs.
			dstAppSpec, defined := base.Data.Applications[srcAppName]
			if !defined {
				base.Data.Applications[srcAppName] = srcAppSpec
				continue
			}

			mergeStructs(dstAppSpec, srcAppSpec, fpm.forField(srcAppName))
		}
	}

	// Merge SAAS blocks
	if len(overlay.Data.Saas) != 0 {
		if base.Data.Saas == nil {
			base.Data.Saas = make(map[string]*SaasSpec, len(overlay.Data.Saas))
		}

		fpm := overlay.PresenceMap.forField("saas")
		for srcSaasName, srcSaasSpec := range overlay.Data.Saas {
			// If the overlay map points to an empty object, delete
			// it from the base bundle
			if isZero(reflect.ValueOf(srcSaasSpec)) {
				delete(base.Data.Saas, srcSaasName)
				base.Data.Relations = removeRelations(base.Data.Relations, srcSaasName)
				continue
			}

			// if this is a new saas block just append it; otherwise
			// recursively merge the two saas specs.
			dstSaasSpec, defined := base.Data.Saas[srcSaasName]
			if !defined {
				base.Data.Saas[srcSaasName] = srcSaasSpec
				continue
			}

			mergeStructs(dstSaasSpec, srcSaasSpec, fpm.forField(srcSaasName))
		}
	}

	// If series is set in the config, it overrides the bundle.
	if series := overlay.Data.Series; series != "" {
		base.Data.Series = series
	}

	// Append any additional relations.
	base.Data.Relations = append(base.Data.Relations, overlay.Data.Relations...)

	// Override machine definitions.
	if machines := overlay.Data.Machines; machines != nil {
		base.Data.Machines = machines
	}

	return nil
}

// removeRelations removes any relation defined in data that references
// the application appName.
func removeRelations(data [][]string, appName string) [][]string {
	var result [][]string
	for _, relation := range data {
		// Keep the dud relation in the set, it will be caught by the bundle
		// verify code.
		if len(relation) == 2 {
			left, right := relation[0], relation[1]
			if left == appName || strings.HasPrefix(left, appName+":") ||
				right == appName || strings.HasPrefix(right, appName+":") {
				continue
			}
		}
		result = append(result, relation)
	}
	return result
}

// mergeStructs iterates the fields of srcStruct and merges them into the
// equivalent fields of dstStruct using the following rules:
//
// - if src defines a value for a scalar or slice field, it will overwrite
//   the value from the dst (e.g. trust, series etc).
// - if the src specifies a nil/empty value for a map field, then the map
//   field of dst will be cleared.
// - if the src specifies a non-empty value for a map field, its key/value
//   tuples are iterated and:
//   - if the value is nil/zero and non-scalar, it is deleted from the dst map.
//   - otherwise, the key/value is inserted into the dst map overwriting any
//     existing entries.
func mergeStructs(dstStruct, srcStruct interface{}, fpm FieldPresenceMap) {
	dst := reflect.ValueOf(dstStruct)
	src := reflect.ValueOf(srcStruct)
	typ := src.Type()

	// Dereference pointers
	if src.Kind() == reflect.Ptr {
		src = src.Elem()
		typ = src.Type()
	}
	if dst.Kind() == reflect.Ptr {
		dst = dst.Elem()
	}
	dstTyp := dst.Type()

	// Sanity check
	if typ.Kind() != reflect.Struct || typ != dstTyp {
		panic(errors.Errorf("BUG: source/destination type mismatch; expected destination to be a %q; got %q", typ.Name(), dstTyp.Name()))
	}

	for i := 0; i < typ.NumField(); i++ {
		// Skip non-exportable fields
		structField := typ.Field(i)
		srcVal := src.Field(i)
		if !srcVal.CanInterface() {
			continue
		}

		fieldName := yamlName(structField)
		if !fpm.fieldPresent(fieldName) {
			continue
		}

		switch srcVal.Kind() {
		case reflect.Map:
			// If a nil/empty map is provided then clear the destination map.
			if isZero(srcVal) {
				dst.Field(i).Set(reflect.MakeMap(srcVal.Type()))
				continue
			}

			dstMap := dst.Field(i)
			if dstMap.IsNil() {
				dstMap.Set(reflect.MakeMap(srcVal.Type()))
			}
			for _, srcKey := range srcVal.MapKeys() {
				// If the key points to an empty non-scalar value delete it from the dst map
				srcMapVal := srcVal.MapIndex(srcKey)
				if isZero(srcMapVal) && isNonScalar(srcMapVal) {
					// Setting an empty value effectively deletes the key from the map
					dstMap.SetMapIndex(srcKey, reflect.Value{})
					continue
				}

				dstMap.SetMapIndex(srcKey, srcMapVal)
			}
		case reflect.Slice:
			dst.Field(i).Set(srcVal)
		default:
			dst.Field(i).Set(srcVal)
		}
	}
}

// isNonScalar returns true if val is a non-scalar value such as a pointer,
// struct, map or slice.
func isNonScalar(val reflect.Value) bool {
	kind := val.Kind()

	if kind == reflect.Interface {
		kind = reflect.TypeOf(val).Kind()
	}

	switch kind {
	case reflect.Ptr, reflect.Struct,
		reflect.Map, reflect.Slice, reflect.Array:
		return true
	default:
		return false
	}
}

// resolveIncludes operates on v which is expected to be string. It checks the
// value for the presence of an include directive. If such a directive is
// located, resolveIncludes invokes the provided includeResolver and returns
// back its output after applying the appropriate encoding for the directive.
func resolveIncludes(includeResolver func(path string) ([]byte, error), v interface{}) (string, bool, error) {
	directives := []struct {
		directive string
		encoder   func([]byte) string
	}{
		{
			directive: "include-file://",
			encoder: func(d []byte) string {
				return string(d)
			},
		},
		{
			directive: "include-base64://",
			encoder:   base64.StdEncoding.EncodeToString,
		},
	}

	val, isString := v.(string)
	if !isString {
		return "", false, nil
	}

	for _, dir := range directives {
		if !strings.HasPrefix(val, dir.directive) {
			continue
		}

		path := val[len(dir.directive):]
		data, err := includeResolver(path)
		if err != nil {
			return "", false, errors.Annotatef(err, "resolving include %q", path)
		}

		return dir.encoder(data), true, nil
	}

	return val, false, nil
}

// resolveRelativeCharmPath resolves charmURL into an absolute path relative
// to basePath if charmURL contains a relative path. Otherwise, the function
// returns back the original charmURL.
//
// Note: this function will only resolve paths. It will not check whether the
// referenced charm path actually exists. That is the job of the bundle
// validator.
func resolveRelativeCharmPath(basePath, charmURL string) (string, error) {
	// We don't need to do anything for non-relative paths.
	if !strings.HasPrefix(charmURL, ".") {
		return charmURL, nil
	}

	return filepath.Abs(filepath.Join(basePath, charmURL))
}
