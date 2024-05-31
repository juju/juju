// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package base

// ControllerBases returns the supported workload bases available to it at the
// execution time.
func ControllerBases() []Base {
	return []Base{
		MakeDefaultBase(UbuntuOS, "20.04"),
		MakeDefaultBase(UbuntuOS, "22.04"),
	}
}

// WorkloadBases returns the supported workload bases available to it at the
// execution time.
func WorkloadBases() []Base {
	return []Base{
		MakeDefaultBase(UbuntuOS, "20.04"),
		MakeDefaultBase(UbuntuOS, "22.04"),
		MakeDefaultBase(UbuntuOS, "23.10"),
		MakeDefaultBase(UbuntuOS, "24.04"),
	}
}
