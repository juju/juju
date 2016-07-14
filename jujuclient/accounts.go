// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"io/ioutil"
	"os"

	"github.com/juju/errors"
	"github.com/juju/utils"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/juju/osenv"
)

// JujuAccountsPath is the location where accounts information is
// expected to be found.
func JujuAccountsPath() string {
	return osenv.JujuXDGDataHomePath("accounts.yaml")
}

// ReadAccountsFile loads all accounts defined in a given file.
// If the file is not found, it is not an error.
func ReadAccountsFile(file string) (map[string]AccountDetails, error) {
	data, err := ioutil.ReadFile(file)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if err := migrateLegacyAccounts(data); err != nil {
		return nil, err
	}
	accounts, err := ParseAccounts(data)
	if err != nil {
		return nil, err
	}
	return accounts, nil
}

// WriteAccountsFile marshals to YAML details of the given accounts
// and writes it to the accounts file.
func WriteAccountsFile(controllerAccounts map[string]AccountDetails) error {
	data, err := yaml.Marshal(accountsCollection{controllerAccounts})
	if err != nil {
		return errors.Annotate(err, "cannot marshal accounts")
	}
	return utils.AtomicWriteFile(JujuAccountsPath(), data, os.FileMode(0600))
}

// ParseAccounts parses the given YAML bytes into accounts metadata.
func ParseAccounts(data []byte) (map[string]AccountDetails, error) {
	var result accountsCollection
	if err := yaml.Unmarshal(data, &result); err != nil {
		return nil, errors.Annotate(err, "cannot unmarshal accounts")
	}
	return result.ControllerAccounts, nil
}

type accountsCollection struct {
	ControllerAccounts map[string]AccountDetails `yaml:"controllers"`
}

// TODO(axw) 2016-07-14 #NNN
// Drop this code once we get to 2.0-beta13.
func migrateLegacyAccounts(data []byte) error {
	type legacyControllerAccounts struct {
		Accounts       map[string]AccountDetails `yaml:"accounts"`
		CurrentAccount string                    `yaml:"current-account,omitempty"`
	}
	type legacyAccountsCollection struct {
		ControllerAccounts map[string]legacyControllerAccounts `yaml:"controllers"`
	}
	var legacy legacyAccountsCollection
	if err := yaml.Unmarshal(data, &legacy); err != nil {
		return errors.Annotate(err, "cannot unmarshal accounts")
	}
	result := make(map[string]AccountDetails)
	for controller, controllerAccounts := range legacy.ControllerAccounts {
		if controllerAccounts.CurrentAccount == "" {
			continue
		}
		details, ok := controllerAccounts.Accounts[controllerAccounts.CurrentAccount]
		if !ok {
			continue
		}
		result[controller] = details
	}
	if len(result) > 0 {
		// Only write if we found at least one,
		// which means the file was in legacy
		// format. Otherwise leave it alone.
		return WriteAccountsFile(result)
	}
	return nil
}
