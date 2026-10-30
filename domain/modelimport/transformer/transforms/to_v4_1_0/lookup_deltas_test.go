// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package to_v4_1_0

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/juju/tc"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/domain/export/types/v4_0_12"
	"github.com/juju/juju/domain/export/types/v4_1_0"
	coretesting "github.com/juju/juju/internal/testing"
)

type lookupDeltasSuite struct {
	coretesting.BaseSuite
}

func TestLookupDeltasSuite(t *testing.T) {
	tc.Run(t, &lookupDeltasSuite{})
}

// Discover the changed lookup shapes so every field of every consumed legacy
// lookup is checked, not just a representative ID/name pair. The fixed count
// prevents a missing target type change from silently reducing coverage.
func (s *lookupDeltasSuite) TestCompleteRows(c *tc.C) {
	source := reflect.TypeFor[v4_0_12.ModelExport]()
	target := reflect.TypeFor[v4_1_0.ModelExport]()
	lookups := 0
	for field := range source.Fields() {
		if field.Name == "ChangeLog" || field.Name == "ChangeLogEditType" || field.Name == "ChangeLogNamespace" {
			continue
		}
		rowType := field.Type.Elem()
		idField, ok := rowType.FieldByName("ID")
		if !ok || idField.Type != reflect.TypeFor[*int64]() {
			continue
		}
		targetField, ok := target.FieldByName(field.Name)
		c.Assert(ok, tc.IsTrue)
		targetID, ok := targetField.Type.Elem().FieldByName("ID")
		c.Assert(ok, tc.IsTrue)
		c.Check(targetID.Type, tc.Equals, reflect.TypeFor[int64]())
		method := reflect.ValueOf(deltas{}).MethodByName(field.Name)
		c.Assert(method.IsValid(), tc.IsTrue)
		lookups++
		c.Logf("checking complete rows for %s", field.Name)

		call := func(rows reflect.Value) (reflect.Value, error) {
			out := method.Call([]reflect.Value{
				reflect.ValueOf(c.Context()), rows, reflect.ValueOf((*v4_0_12.ModelExport)(nil)),
			})
			if !out[1].IsNil() {
				return out[0], out[1].Interface().(error)
			}
			return out[0], nil
		}

		for _, ids := range [][]int64{{0}, {7}, {0, 7, -9}} {
			// Exercise populated and NULL non-ID attributes independently.
			for _, nullable := range []bool{false, true} {
				rows := reflect.MakeSlice(field.Type, len(ids), len(ids))
				want := reflect.MakeSlice(targetField.Type, len(ids), len(ids))
				for rowIndex, id := range ids {
					row := rows.Index(rowIndex)
					row.FieldByName("ID").Set(reflect.ValueOf(new(id)))
					want.Index(rowIndex).FieldByName("ID").SetInt(id)
					for column := 0; column < rowType.NumField(); column++ {
						name := rowType.Field(column).Name
						if name == "ID" {
							continue
						}
						value := row.Field(column)
						text := fmt.Sprintf("%s-%s-%d", field.Name, name, rowIndex)
						if value.Kind() == reflect.Pointer {
							if nullable {
								value.Set(reflect.ValueOf(new(text)))
							}
						} else {
							value.SetString(text)
						}
						want.Index(rowIndex).FieldByName(name).Set(value)
					}
				}
				got, err := call(rows)
				c.Assert(err, tc.ErrorIsNil)
				c.Check(got.Interface(), tc.DeepEquals, want.Interface())
			}
		}

		for _, badIndex := range []int{0, 1, 2} {
			rows := reflect.MakeSlice(field.Type, 3, 3)
			for rowIndex := 0; rowIndex < rows.Len(); rowIndex++ {
				if rowIndex != badIndex {
					rows.Index(rowIndex).FieldByName("ID").Set(reflect.ValueOf(new(int64(rowIndex))))
				}
			}
			got, err := call(rows)
			c.Assert(err, tc.ErrorIs, coreerrors.NotValid)
			c.Check(err, tc.ErrorMatches, fmt.Sprintf("invalid %s row %d: id is NULL", field.Tag.Get("yaml"), badIndex))
			c.Check(got.IsNil(), tc.IsTrue)
		}

		got, err := call(reflect.MakeSlice(field.Type, 0, 0))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(got.Len(), tc.Equals, 0)
	}
	c.Check(lookups, tc.Equals, 45)
}

func (s *lookupDeltasSuite) TestTransformPreservesTargetLocalExclusions(c *tc.C) {
	got, err := NewTransform(NewDeltas())(c.Context(), v4_0_12.ModelExport{
		ChangeLogEditType:  []v4_0_12.ChangeLogEditType{{EditType: new("legacy")}},
		ChangeLogNamespace: []v4_0_12.ChangeLogNamespace{{Namespace: new("legacy"), Description: new("legacy")}},
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(got.ChangeLogEditType, tc.HasLen, 0)
	c.Check(got.ChangeLogNamespace, tc.HasLen, 0)
	c.Check(got.SshKeyAlgorithmType, tc.DeepEquals, []v4_1_0.SshKeyAlgorithmType{
		{ID: 0, Type: "ssh-rsa"},
		{ID: 1, Type: "ecdsa-sha2-nistp256"},
		{ID: 2, Type: "ssh-ed25519"},
	})
}
