// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// This package contains a mock implementation of the manager.PackageManager
// interface which always returns positive outcomes and a nil error.
package testing

import "github.com/juju/utils/proxy"

// MockPackageManager is a struct which always returns a positive outcome,
// constant ProxySettings and a nil error.
// It satisfies the PackageManager interface.
type MockPackageManager struct {
}

// InstallPrerequisite is defined on the PackageManager interface.
func (pm *MockPackageManager) InstallPrerequisite() error {
	return nil
}

// Update is defined on the PackageManager interface.
func (pm *MockPackageManager) Update() error {
	return nil
}

// Upgrade is defined on the PackageManager interface.
func (pm *MockPackageManager) Upgrade() error {
	return nil
}

// Install is defined on the PackageManager interface.
func (pm *MockPackageManager) Install(...string) error {
	return nil
}

// Remove is defined on the PackageManager interface.
func (pm *MockPackageManager) Remove(...string) error {
	return nil
}

// Purge is defined on the PackageManager interface.
func (pm *MockPackageManager) Purge(...string) error {
	return nil
}

// Search is defined on the PackageManager interface.
func (pm *MockPackageManager) Search(string) (bool, error) {
	return true, nil
}

// IsInstalled is defined on the PackageManager interface.
func (pm *MockPackageManager) IsInstalled(string) bool {
	return true
}

// AddRepository is defined on the PackageManager interface.
func (pm *MockPackageManager) AddRepository(string) error {
	return nil
}

// RemoveRepository is defined on the PackageManager interface.
func (pm *MockPackageManager) RemoveRepository(string) error {
	return nil
}

// Cleanup is defined on the PackageManager interface.
func (pm *MockPackageManager) Cleanup() error {
	return nil
}

// GetProxySettings is defined on the PackageManager interface.
func (pm *MockPackageManager) GetProxySettings() (proxy.Settings, error) {
	return proxy.Settings{"http proxy", "https proxy", "ftp proxy", "no proxy"}, nil
}

// SetProxy is defined on the PackageManager interface.
func (pm *MockPackageManager) SetProxy(proxy.Settings) error {
	return nil
}
