// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package annotation

// Annotation domain package provides the service that keeps track of
// annotations at the model level. Annotations are key/value pairs that can be
// associated with any annotatable entity in the model (see core/annotations
// package for the kind of entities that can be annotated).
//
// The service provides GetAnnotations and SetAnnotations methods to retrieve
// and set annotations respectively for a given ID. See the service and state
// packages for details about their api.
//
//
// GetAnnotations retrieves all the annotations associated with a given ID.
// If no annotations are found, an empty map is returned.
// GetAnnotations(ctx context.Context, id annotations.ID) (map[string]string,
// error)
//
//
// SetAnnotations associates key/value annotation pairs with a given ID.
// If annotation already exists for the given ID, then it will be updated
// with the given value.
// SetAnnotations(ctx context.Context, id annotations.ID, annotations
// map[string]string) error
//
//
// See core/annotations package for the definition of the ID type.
//
// This domain is accessed and used only by the Annotations facade on the API
// server, for the Get and Set methods (see apiserver/facades/client/annotations
// for details), both supporting bulk operations.
