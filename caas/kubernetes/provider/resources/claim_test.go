// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources

import (
	stdtesting "testing"

	"github.com/juju/errors"
	core "k8s.io/api/core/v1"
	rbac "k8s.io/api/rbac/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestClaimHasJujuLabel(t *stdtesting.T) {
	tests := []struct {
		Name   string
		Obj    interface{}
		Result bool
	}{
		{
			Name: "Test that object has a single juju label",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"juju.io/something": "some-value",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test that object has no juju labels",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"does-not-mention-secret-keys": "foo",
					},
				},
			},
			Result: false,
		},
		{
			Name: "Test many labels with juju key",
			Obj: &rbac.ClusterRole{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"label1":     "foo",
						"label2":     "foo",
						"label3":     "foo",
						"label4":     "foo",
						"label5":     "foo",
						"label6":     "foo",
						"label7":     "foo",
						"label8":     "foo",
						"label9":     "foo",
						"juju-model": "AA==",
					},
				},
			},
			Result: true,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *stdtesting.T) {
			r, err := claimHasJujuLabel(test.Obj)
			if err != nil {
				t.Errorf("unexpected error testing claim has juju label %v", err)
			}
			if r != test.Result {
				t.Errorf("unexpected result for claim has juju label, weanted %t got %t",
					test.Result, r)
			}
		})
	}
}

func TestClaimHasJujuLabelBadData(t *stdtesting.T) {
	r, err := claimHasJujuLabel(map[string]string{})
	if r {
		t.Error("expected claim has juju label with bad data returns false")
	}

	if err == nil {
		t.Error("expected claim has juju label with bad data returns an error")
	}
}

func TestClaimHasJujuLabelNilData(t *stdtesting.T) {
	r, err := claimHasJujuLabel(nil)
	if r {
		t.Error("expected claim has juju label with nil data returns false")
	}

	if !errors.Is(err, errors.NotValid) {
		t.Error("expected claim has juju label with nil to be a not valid error")
	}
}

func TestClaimIsManagedByJuju(t *stdtesting.T) {
	tests := []struct {
		Name   string
		Obj    interface{}
		Result bool
	}{
		{
			Name: "Test that object is not managed by juju",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"juju.io/something": "some-value",
					},
				},
			},
			Result: false,
		},
		{
			Name: "Test that object is managed by juju",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "juju",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test many labels with is managed by juju",
			Obj: &rbac.ClusterRole{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"label1":                       "foo",
						"label2":                       "foo",
						"label3":                       "foo",
						"label4":                       "foo",
						"label5":                       "foo",
						"label6":                       "foo",
						"label7":                       "foo",
						"label8":                       "foo",
						"label9":                       "foo",
						"app.kubernetes.io/managed-by": "juju",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test not managed by Juju",
			Obj: &rbac.ClusterRole{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"label1":                       "foo",
						"label2":                       "foo",
						"label3":                       "foo",
						"label4":                       "foo",
						"label5":                       "foo",
						"label6":                       "foo",
						"label7":                       "foo",
						"label8":                       "foo",
						"label9":                       "foo",
						"app.kubernetes.io/managed-by": "notjuju",
					},
				},
			},
			Result: false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *stdtesting.T) {
			r, err := claimIsManagedByJuju(test.Obj)
			if err != nil {
				t.Errorf("unexpected error testing is managed by juju %v", err)
			}
			if r != test.Result {
				t.Errorf("unexpected result for claim is managed by juju, weanted %t got %t",
					test.Result, r)
			}
		})
	}
}

func TestClaimIsManagedByJujuBadData(t *stdtesting.T) {
	r, err := claimHasJujuLabel(map[string]string{})
	if r {
		t.Error("expected claim is managed by juju with bad data returns false")
	}

	if err == nil {
		t.Error("expected claim is managed by juju with bad data returns an error")
	}
}

func TestClaimIsManagedByJujuNilData(t *stdtesting.T) {
	r, err := claimHasJujuLabel(nil)
	if r {
		t.Error("expected claim is managed by juju with nil data returns false")
	}

	if !errors.Is(err, errors.NotValid) {
		t.Error("expected claim is managed by juju with nil to be a not valid error")
	}
}

func TestClaimOrAggregateWithEmptyClaimsReturnsFalse(t *stdtesting.T) {
	r, err := ClaimAggregateOr().Assert(nil)
	if err != nil {
		t.Errorf("unexpected error for empty claim aggregate or %v", err)
	}
	if r {
		t.Errorf("expected empty claim aggregate or to return false")
	}
}

func TestClaimAggregateOrReturnsTrue(t *stdtesting.T) {
	r, err := ClaimAggregateOr(
		ClaimFn(func(_ interface{}) (bool, error) {
			return false, nil
		}),
		ClaimFn(func(_ interface{}) (bool, error) {
			return true, nil
		}),
	).Assert(nil)
	if err != nil {
		t.Errorf("unexpected error for claim aggregate or %v", err)
	}
	if !r {
		t.Errorf("expected claim aggregate or to return true")
	}
}

func TestClaimAggregateOrReturnsFalse(t *stdtesting.T) {
	r, err := ClaimAggregateOr(
		ClaimFn(func(_ interface{}) (bool, error) {
			return false, nil
		}),
		ClaimFn(func(_ interface{}) (bool, error) {
			return false, nil
		}),
	).Assert(nil)
	if err != nil {
		t.Errorf("unexpected error for claim aggregate or %v", err)
	}
	if r {
		t.Errorf("expected claim aggregate or to return false")
	}
}

func TestClaimAggregateOrReturnsError(t *stdtesting.T) {
	r, err := ClaimAggregateOr(
		ClaimFn(func(_ interface{}) (bool, error) {
			return false, nil
		}),
		ClaimFn(func(_ interface{}) (bool, error) {
			return false, errors.New("some-error")
		}),
	).Assert(nil)
	if err == nil {
		t.Error("expected claim aggregate or to return error")
	}
	if r {
		t.Errorf("expected claim aggregate or to return false")
	}
}

func TestClaimJujuOwnership(t *stdtesting.T) {
	tests := []struct {
		Name   string
		Obj    interface{}
		Result bool
	}{
		{
			Name: "Test that object has a single juju label",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"juju.io/something": "some-value",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test that object has no juju labels",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"does-not-mention-secret-keys": "foo",
					},
				},
			},
			Result: false,
		},
		{
			Name: "Test many labels with juju key",
			Obj: &rbac.ClusterRole{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"label1":     "foo",
						"label2":     "foo",
						"label3":     "foo",
						"label4":     "foo",
						"label5":     "foo",
						"label6":     "foo",
						"label7":     "foo",
						"label8":     "foo",
						"label9":     "foo",
						"juju-model": "AA==",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test that object is managed by juju",
			Obj: &core.ConfigMap{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "juju",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test many labels with is managed by juju",
			Obj: &rbac.ClusterRole{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"label1":                       "foo",
						"label2":                       "foo",
						"label3":                       "foo",
						"label4":                       "foo",
						"label5":                       "foo",
						"label6":                       "foo",
						"label7":                       "foo",
						"label8":                       "foo",
						"label9":                       "foo",
						"app.kubernetes.io/managed-by": "juju",
					},
				},
			},
			Result: true,
		},
		{
			Name: "Test not managed by Juju",
			Obj: &rbac.ClusterRole{
				ObjectMeta: meta.ObjectMeta{
					Labels: map[string]string{
						"label1":                       "foo",
						"label2":                       "foo",
						"label3":                       "foo",
						"label4":                       "foo",
						"label5":                       "foo",
						"label6":                       "foo",
						"label7":                       "foo",
						"label8":                       "foo",
						"label9":                       "foo",
						"app.kubernetes.io/managed-by": "notjuju",
					},
				},
			},
			Result: false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *stdtesting.T) {
			r, err := ClaimJujuOwnership.Assert(test.Obj)
			if err != nil {
				t.Errorf("unexpected error testing claim juju ownership %v", err)
			}
			if r != test.Result {
				t.Errorf("unexpected result for claim juju ownership, weanted %t got %t",
					test.Result, r)
			}
		})
	}
}
