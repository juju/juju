// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jujuclient

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
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
	accounts, err := ParseAccounts(data)
	if err != nil {
		return nil, err
	}
	if err := migrateLocalAccountUsers(accounts); err != nil {
		return nil, err
	}
	return accounts, nil
}

func migrateLocalAccountUsers(accounts map[string]AccountDetails) error {
	changes := false
	for user, account := range accounts {
		if !strings.HasSuffix(account.User, "@local") {
			continue
		}
		tag := names.NewUserTag(account.User)
		updated := account
		updated.User = tag.Id()
		accounts[user] = updated
		changes = true
	}
	if changes {
		return WriteAccountsFile(accounts)
	}
	return nil
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
