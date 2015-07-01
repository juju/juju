// Copyright 2014-2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package operation

// DoesNotRequireMachineLock is embedded in the various operations to express whether
// they need a global machine lock or not.
type RequiresMachineLock struct{}

// NeedsGlobalMachineLock is part of the Operation interface.
// It is embedded in the various operations.
func (RequiresMachineLock) NeedsGlobalMachineLock() bool { return true }

// DoesNotRequireMachineLock is embedded in the various operations to express whether
// they need a global machine lock or not.
type DoesNotRequireMachineLock struct{}

// NeedsGlobalMachineLock is part of the Operation interface.
// It is embedded in the various operations.
func (DoesNotRequireMachineLock) NeedsGlobalMachineLock() bool { return false }
