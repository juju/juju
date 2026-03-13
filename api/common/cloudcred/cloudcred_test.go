// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudcred

import (
	"testing"
)

func TestIsVisibleAttribute(t *testing.T) {

	if IsVisibleAttribute("ec2", "access-key", "access-key") != true {
		t.Errorf("expected true, got false")
	}
	if IsVisibleAttribute("ec2", "access-key", "secret-key") != false {
		t.Errorf("expected false, got true")
	}
	if IsVisibleAttribute("ec2", "unknown-auth", "access-key") != false {
		t.Errorf("expected false, got true")
	}
}
