#!/usr/bin/env python
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
import logging
import os
from subprocess import CalledProcessError
import sys

from deploy_stack import BootstrapManager
from jujupy import pause
from utility import (
    JujuResourceTimeout,
    add_basic_testing_arguments,
    configure_logging,
    until_timeout,
)


__metaclass__ = type


log = logging.getLogger("assess_model_migration")


def assess_model_migration(bs1, bs2, upload_tools):
    ensure_able_to_migrate_model_between_controllers(bs1, bs2, upload_tools)
    ensure_fail_to_migrate_to_lower_version_controller(bs1, bs2, upload_tools)


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test model migration feature"
    )
    add_basic_testing_arguments(parser)
    return parser.parse_args(argv)


def get_bootstrap_managers(args):
    """Create 2 bootstrap managers from the provided args.

    Need to make a couple of elements uniqe (e.g. environment name) so we can
    have 2 bootstrapped at the same time.

    """
    bs_1 = BootstrapManager.from_args(args)
    bs_2 = BootstrapManager.from_args(args)

    # Need to be able to upgrade this controller.
    try:
        bs_1.client.env.config.pop('enable-os-upgrade')
    except KeyError:
        pass

    # Give the second a separate/unique name.
    bs_2.temp_env_name = '{}-b'.format(bs_1.temp_env_name)

    bs_1.log_dir = _new_log_dir(args.logs, 'a')
    bs_2.log_dir = _new_log_dir(args.logs, 'b')

    return bs_1, bs_2


def _new_log_dir(log_dir, post_fix):
    new_log_dir = os.path.join(log_dir, 'env-{}'.format(post_fix))
    os.mkdir(new_log_dir)
    return new_log_dir


def wait_for_model(client, model_name):
    for _ in until_timeout(60):
        models = client.get_models()
        if model_name in [m['name'] for m in models['models']]:
            return
        pause(1)
    raise JujuResourceTimeout()


def _get_controllers_version(client):
    status_details = client.get_status()
    return status_details.status['machines']['0']['juju-status']['version']


def _get_admin_client(client):
    return client.clone(client.env.clone('admin'))


def _update_client_controller(client):
    log.info('Updating clients ({}) controller'.format(
        client.env.environment))
    admin_client = _get_admin_client(client)
    original_version = _get_controllers_version(admin_client)
    admin_client.juju('upgrade-juju', ('--upload-tools'))

    # Wait for update to happen on controller
    for _ in until_timeout(60):
        if original_version == _get_controllers_version(admin_client):
            return
    raise RuntimeError('Failed to update controller.')


def test_deployed_mongo_is_up(client):
    """Ensure the mongo service is running as expected."""
    try:
        output = client.get_juju_output(
            'ssh', 'mongodb/0', 'mongo --eval "db.getMongo()"')
        if 'connecting to: test' in output:
            return
    except CalledProcessError as e:
        # Pass through to assertion error
        log.error('Mongodb check command failed: {}'.format(e))
    raise AssertionError('Mongo db is not in an expected state.')


def ensure_able_to_migrate_model_between_controllers(bs1, bs2, upload_tools):
    with bs1.booted_context(upload_tools):
        bs1.client.enable_feature('migration')

        log.info('Deploying charm')
        bs1.client.juju("deploy", ('mongodb'))
        bs1.client.wait_for_started()
        test_deployed_mongo_is_up(bs1.client)

        log.info('Booting second instance')
        bs2.client.env.juju_home = bs1.client.env.juju_home
        with bs2.existing_booted_context(upload_tools):
            log.info('Initiating migration process')

            bs1.client.controller_juju(
                'migrate',
                (bs1.client.env.environment,
                 'local.{}'.format(bs2.client.env.controller.name)))

            migration_target_client = bs2.client.clone(
                bs2.client.env.clone(bs1.client.env.environment))

            wait_for_model(migration_target_client, bs1.client.env.environment)

            # WIP logging
            migration_target_client.juju('status', ())

            migration_target_client.wait_for_started()
            test_deployed_mongo_is_up(migration_target_client)


def ensure_fail_to_migrate_to_lower_version_controller(bs1, bs2, upload_tools):
    """
    Migration must not proceed if the target controller version is less than
    the source controller.

    """
    with bs1.booted_context(upload_tools):
        bs1.client.enable_feature('migration')

        _update_client_controller(bs1.client)

        log.info('Booting second instance')
        bs2.client.env.juju_home = bs1.client.env.juju_home
        with bs2.existing_booted_context(upload_tools):
            log.info('Initiating migration process')
            try:
                bs1.client.controller_juju(
                    'migrate',
                    (bs1.client.env.environment,
                     'local.{}'.format(bs2.client.env.controller.name)))
            except CalledProcessError as e:
                if 'expected error message' in e:
                    return True
            # If the migration didn't fail there is an issue and we need to
            # fail
            raise RuntimeError(
                'Migrating to upgraded controller did not error.')


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    bs1, bs2 = get_bootstrap_managers(args)

    assess_model_migration(bs1, bs2, args.upload_tools)

    return 0


if __name__ == '__main__':
    sys.exit(main())
