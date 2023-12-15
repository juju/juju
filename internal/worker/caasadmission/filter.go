// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasadmission

import (
	apis "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// admissionObjectIgnores defines a slice of GVK's that should be ignored by
// the caasadmission controller.
var admissionObjectIgnores = []apis.GroupVersionKind{
	// ignoring SelfSubjectAccessReview checks because of bug lp-1910989
	{
		Group:   "authorization.k8s.io",
		Kind:    "SelfSubjectAccessReview",
		Version: "v1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "SelfSubjectRulesReview",
		Version: "v1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "SelfSubjectAccessReview",
		Version: "v1beta1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "SelfSubjectRulesReview",
		Version: "v1beta1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "SubjectAccessReview",
		Version: "v1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "SubjectAccessReview",
		Version: "v1beta1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "LocalSubjectAccessReview",
		Version: "v1",
	},
	{
		Group:   "authorization.k8s.io",
		Kind:    "LocalSubjectAccessReview",
		Version: "v1beta1",
	},
}

// compareAPIGroupVersionKind compares two api GroupVersionKind objects for
// eqauoity.
func compareAPIGroupVersionKind(a apis.GroupVersionKind, b apis.GroupVersionKind) bool {
	return a.Group == b.Group && a.Kind == b.Kind && a.Version == b.Version
}
