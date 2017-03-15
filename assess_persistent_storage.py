#!/usr/bin/env python
"""Testing persistent storage function of Juju."""

from __future__ import print_function

import argparse
import yaml
import logging
import sys

from deploy_stack import (
    BootstrapManager,
    get_random_string,
    )
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    )
from jujucharm import local_charm_path

__metaclass__ = type


log = logging.getLogger("assess_persistent_storage")


def assess_persistent_storage(client):
    ensure_storage_remains_after_application_removal()


def ensure_storage_remains_after_application_removal(client):
    """Storage created during a deploy must persist after application removal.

    Steps taken to test:
      - Deploy the dummy storage charm
      - Set config and ensure data is stored on storage.
      - Remove application, taking note of the remaining storage name
      - Re-deploy new charm using persisted storage
      - Ensure data has remained.

    :param client: ModelClient object to deploy the charm on.
    """
    random_token = get_random_string()
    expected_token_values = {'single-fs-token': random_token}
    charm_path = local_charm_path(
        charm='dummy-storage', juju_ver=client.version)
    client.deploy(charm_path, storage='single-fs=rootfs')
    client.wait_for_started()
    client.set_config('dummy-storage', {'single-fs-token': random_token})
    client.wait_for_workloads()

    assert_storage_is_intact(client, expected_results=expected_token_values)

    try:
        single_filesystem_name = get_storage_filesystems(
            client, 'single-fs')[0]
    except IndexError:
        raise JujuAssertionError('Storage was not found.')

    client.remove_service('dummy-storage')

    # Wait for application to be removed then re-deploy with existing storage.

    storage_command = '--attach-storage single-fs={}'.format(
        single_filesystem_name)
    client.deploy(charm_path, alias=storage_command)
    client.wait_for_started()
    client.wait_for_workloads()

    assert_storage_is_intact(client, expected_token_values)


def get_storage_filesystems(client, storage_name):
    """Return storage unit names for a named storage.

    :param client: ModelClient object to query.
    :param storage_name: Name of storage unit to get filesystem names for.
    :return: List of filesystem names
    """
    all_storage = yaml.safe_load(client.list_storage())['filesystems']
    return [
        details['storage'] for unit, details in all_storage.items()
        if details['storage'].startswith(storage_name)]


def assert_storage_is_intact(client, expected_results):
    """Ensure stored tokens match the expected values in `expected_results`.

    Checks the token values stored on the storage assigned to the deployed
    dummy-storage charm.

    Only matches the provided expected token values and ignores any that have
    no values provided for them.

    :param client: ModelClient object where dummy-storage application is
      deployed
    :param expected_results: Dict containing 'token name' -> 'expected value'
      look up values.
    """
    pass


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test for Persistent Storage feature.")
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    with bs_manager.booted_context(args.upload_tools):
        assess_persistent_storage(bs_manager.client)
    return 0


if __name__ == '__main__':
    sys.exit(main())
