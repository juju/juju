#!/usr/bin/env python
"""Test migrating models between controllers of increasing versions."""

from __future__ import print_function

import argparse
from distutils.version import (
    LooseVersion,
    StrictVersion
    )
import logging
import os
import sys

from assess_model_migration import (
    _new_log_dir,
    assert_model_migrated_successfully,
    deploy_simple_server_to_new_model,
    migrate_model_to_controller,
)
from jujupy.client import get_stripped_version_number
from deploy_stack import (
    BootstrapManager,
    client_from_config,
    get_random_string,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    )

__metaclass__ = type

log = logging.getLogger('assess_model_migration_versions')


def assess_model_migration_versions(stable_bsm, devel_bsm, args):
    with stable_bsm.booted_context(args.upload_tools):
        stable_bsm.client.enable_feature('migration')
        devel_bsm.client.enable_feature('migration')
        devel_bsm.client.env.juju_home = stable_bsm.client.env.juju_home
        with devel_bsm.existing_booted_context(args.upload_tools):
            # Create model with stuff in it.
            stable_client = stable_bsm.client
            devel_client = devel_bsm.client
            resource_contents = get_random_string()
            test_stable_model, application = deploy_simple_server_to_new_model(
                stable_client, 'version-migration', resource_contents)
            migration_target_client = migrate_model_to_controller(
                test_stable_model, devel_client)
            assert_model_migrated_successfully(
                migration_target_client, application, resource_contents)


def get_stable_juju(args, stable_juju_bin=None):
    """Get the installed stable version of juju.

    We need a stable version of juju to boostrap and migrate from to the newer
    development version of juju.

    If no juju path is provided try some well known paths in an attempt to find
    a system installed juju that will suffice.
    Note. this function does not check if the found juju is a suitable version
    for this test, just that the binary exists and is executable.

    :param stable_juju_bin: Path to the juju binary to be used and considered
      stable
    :raises RuntimeError: If there is no valid installation of juju available.
    :return: BootstrapManager object for the stable juju.
    """
    if stable_juju_bin is not None:
        try:
            client = client_from_config(
                args.env,
                stable_juju_bin,
                debug=args.debug)
            log.info('Using {} for stable juju'.format(stable_juju_bin))
            return BootstrapManager.from_client(args, client)
        except OSError as e:
            raise RuntimeError(
                'Provided stable juju path is not valid: {}'.format(e))
    known_juju_paths = (
        '/snap/bin/juju',
        '/usr/bin/juju',
        '{}/bin/juju'.format(os.environ.get('GOPATH')))

    for path in known_juju_paths:
        try:
            client = client_from_config(
                args.env,
                path,
                debug=args.debug)
            log.info('Using {} for stable juju'.format(path))
            return BootstrapManager.from_client(args, client)
        except OSError:
            log.debug('Attempt at using {} failed.'.format(path))
            pass

    raise RuntimeError('Unable to get a stable system juju binary.')


def assert_stable_juju_suitable_for_testing(stable_bsm, devel_bsm):
    """Stable juju must be an earlier version than devel."""
    stable_version = get_stripped_version_number(stable_bsm.client.version)
    dev_version = get_stripped_version_number(devel_bsm.client.version)
    try:
        dev_newer = StrictVersion(dev_version) >= StrictVersion(stable_version)
    except ValueError:
        dev_newer = LooseVersion(dev_version) >= LooseVersion(stable_version)
    if not dev_newer:
        raise RuntimeError(
            'Stable juju "{}"is more recent than develop "{}"'.format(
                stable_version, dev_version))


def parse_args(argv):
    parser = argparse.ArgumentParser(
        description='Test model migration between versioned controllers.')
    add_basic_testing_arguments(parser)
    parser.add_argument(
        '--stable-juju-bin',
        help='Path to juju binary to be used as the stable version of juju.')
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    stable_bsm = get_stable_juju(args, args.stable_juju_bin)
    devel_bsm = BootstrapManager.from_args(args)

    assert_stable_juju_suitable_for_testing(stable_bsm, devel_bsm)

    # Need to make the bootstrap envs unique.
    stable_bsm.temp_env_name = '{}-stable'.format(stable_bsm.temp_env_name)
    devel_bsm.temp_env_name = '{}-devel'.format(devel_bsm.temp_env_name)
    stable_bsm.log_dir = _new_log_dir(stable_bsm.log_dir, 'stable')
    devel_bsm.log_dir = _new_log_dir(stable_bsm.log_dir, 'devel')

    assess_model_migration_versions(stable_bsm, devel_bsm, args)


if __name__ == '__main__':
    sys.exit(main())
