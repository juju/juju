// Copyright 2024 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package errors

import (
	"errors"
	"fmt"
	"testing"
)

func ExampleConstError() {
	notFound := ConstError("not found")

	err := fmt.Errorf("flux capacitor %w", notFound)
	fmt.Println(errors.Is(err, notFound))
	fmt.Println(notFound == ConstError("not found"))
	// Output:
	// true
	// true
}

// TestConstErrorComparable is asserting the properties of a ConstError and that
// two ConstErrors with the same value are comparable to true and satisfy Is().
func TestConstErrorComparable(t *testing.T) {
	comp1 := ConstError("cannot compute value in the future")
	comp2 := ConstError("cannot compute value in the future")

	if comp1 != comp2 {
		t.Errorf("ConstError %q is not equal to ConstError %q", comp1, comp2)
	}

	if !Is(comp1, comp2) {
		t.Errorf("ConstError %q Is() not ConstError %q", comp1, comp2)
	}
}

// TestConstErrorNotComparable is asserting the properties of a ConstError and
// that two ConstErrors with different values are not comparable and don't
// satisfy Is().
func TestConstErrorNotComparable(t *testing.T) {
	comp1 := ConstError("not your error")
	comp2 := ConstError("not the same error")

	if comp1 == comp2 {
		t.Errorf("ConstError %q is equal to ConstError %q", comp1, comp2)
	}

	if Is(comp1, comp2) {
		t.Errorf("ConstError %q Is() ConstError %q", comp1, comp2)
	}
}

// TestConstErrorError asserts that the value used to construct a ConstError is
// the same comparable value that is returned via the Error() func.
func TestConstErrorError(t *testing.T) {
	comp1 := ConstError("zerocool")

	if comp1.Error() != "zerocool" {
		t.Errorf("ConstError %q Error() method did not return \"zerocool\"", comp1)
	}
}

// TestConstErrorPrinting asserts that when using fmt style functions with
// ConstError the value it uses is not altered from that of the original value
// it was constructed with.
func TestConstErrorPrinting(t *testing.T) {
	comp1 := ConstError("printable error")
	val := fmt.Sprintf("%v", comp1)
	if val != "printable error" {
		t.Errorf("ConstError %q printing with %%v was not equal to \"printable error\"", comp1)
	}

	err := fmt.Errorf("%w", comp1)
	if err.Error() != "printable error" {
		t.Errorf("ConstError %q printing with %%w was not equal to \"printable error\"", comp1)
	}
}
