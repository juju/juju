#!/usr/bin/env python
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
import logging
import os
from subprocess import CalledProcessError
import sys
from time import sleep

from deploy_stack import (
    BootstrapManager,
    assess_upgrade,
)
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    until_timeout,
)


__metaclass__ = type


log = logging.getLogger("assess_model_migration")


def assess_model_migration(bs1, bs2, upload_tools):
    # ensure_able_to_migrate_model_between_controllers(bs1, bs2, upload_tools)
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
    bs_1.client.env.config['enable-os-upgrade'] = True

    # Give the second a separate/unique name.
    bs_2.temp_env_name = '{}-b'.format(bs_1.temp_env_name)

    bs_1.log_dir = _new_log_dir(args.logs, 'a')
    bs_2.log_dir = _new_log_dir(args.logs, 'b')

    return bs_1, bs_2


def _new_log_dir(log_dir, post_fix):
    new_log_dir = os.path.join(log_dir, 'env-{}'.format(post_fix))
    os.mkdir(new_log_dir)
    return new_log_dir


def wait_for_model(client, model_name, timeout=60):
    """Wait for a given `timeout` for a model of `model_name` to appear within
    `client`.

    Defaults to 10 seconds timeout.
    :raises AssertionError: If the named model does not appear in the specified
      timeout.

    """
    for _ in until_timeout(timeout):
        models = client.get_models()
        if model_name in [m['name'] for m in models['models']]:
            return
        sleep(1)
    raise JujuAssertionError(
        'Model \'{}\' failed to appear after {} seconds'.format(
            model_name, timeout
        ))


def _update_client_controller(client):
    log.info('Updating clients ({}) controller'.format(
        client.env.environment))

    admin_client = client.get_controller_client()
    admin_client.env.local = True
    admin_client.upgrade_controller()
    admin_client.wait_for_version(
        admin_client.get_matching_agent_version(), 600)
    # assess_upgrade(admin_client, admin_client.full_path)
    # After upgrade, is there an exception perhaps?


def test_deployed_mongo_is_up(client):
    """Ensure the mongo service is running as expected."""
    try:
        output = client.get_juju_output(
            'run', '--unit', 'mongodb/0', 'mongo --eval "db.getMongo()"')
        if 'connecting to: test' in output:
            return
    except CalledProcessError as e:
        # Pass through to assertion error
        log.error('Mongodb check command failed: {}'.format(e))
    raise AssertionError('Mongo db is not in an expected state.')


def ensure_able_to_migrate_model_between_controllers(
        source_environ, dest_environ, upload_tools):
    """Test simple migration of a model to another controller.

    Ensure that migration a model that has an application deployed upon it is
    able to continue it's operation after the migration process.

    Given 2 bootstrapped environments:
      - Deploy an application
        - ensure it's operating as expected
      - Migrate that model to the other environment
        - Ensure it's operating as expected
        - Add a new unit to the application to ensure the model is functional
      - Migrate the model back to the original environment
        - Note: Test for lp:1607457
        - Ensure it's operating as expected
        - Add a new unit to the application to ensure the model is functional


    """
    with source_environ.booted_context(upload_tools):
        source_environ.client.enable_feature('migration')

        bundle = 'mongodb'
        application = 'mongodb'

        log.info('Deploying charm')
        source_environ.client.juju("deploy", (bundle))
        source_environ.client.wait_for_started()
        test_deployed_mongo_is_up(source_environ.client)

        log.info('Booting second instance')
        dest_environ.client.env.juju_home = source_environ.client.env.juju_home
        with dest_environ.existing_booted_context(upload_tools):
            log.info('Initiating migration process')

            migration_target_client = migrate_model_to_controller(
                source_environ, dest_environ)

            test_deployed_mongo_is_up(migration_target_client)
            ensure_model_is_functional(migration_target_client, application)


def migrate_model_to_controller(source_environ, dest_environ):
    source_environ.client.controller_juju(
        'migrate',
        (source_environ.client.env.environment,
         dest_environ.client.env.controller.name))

    migration_target_client = dest_environ.client.clone(
        dest_environ.client.env.clone(
            source_environ.client.env.environment))

    wait_for_model(
        migration_target_client, source_environ.client.env.environment)

    # For logging purposes
    migration_target_client.show_status()
    migration_target_client.wait_for_started()

    return migration_target_client


def ensure_model_is_functional(client, application):
    """Ensures that the migrated model is functional

    Add unit to application to ensure the model is contactable and working.
    Ensure that added unit is created on a new machine (check for bug
    LP:1607599)

    """
    client.juju('add-unit', (application,))
    client.wait_for_started()

    assert_units_on_different_machines(client, application)


def assert_units_on_different_machines(client, application):
    status = client.get_status()
    unit_machines = [u[1]['machine'] for u in status.iter_units()]

    raise_if_shared_machines(unit_machines)


def raise_if_shared_machines(unit_machines):
    """Raise an exception if `unit_machines` contain double ups of machine ids.

    A unique list of machine ids will be equal in length to the set of those
    machine ids.

    :raises ValueError: if an empty list is passed in.
    :raises JujuAssertionError: if any double-ups of machine ids are detected.

    """
    if not unit_machines:
        raise ValueError('Cannot share 0 machines. Empty list provided.')
    if len(unit_machines) != len(set(unit_machines)):
        raise JujuAssertionError('Appliction units reside on the same machine')


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
