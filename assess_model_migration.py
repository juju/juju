#!/usr/bin/env python
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
import logging
import os
from subprocess import CalledProcessError
import sys
from time import sleep

from assess_user_grant_revoke import User
from deploy_stack import BootstrapManager
from jujucharm import local_charm_path
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    temp_dir,
    until_timeout,
)


__metaclass__ = type


log = logging.getLogger("assess_model_migration")


def assess_model_migration(bs1, bs2, upload_tools):
    with bs1.booted_context(upload_tools):
        bs1.client.enable_feature('migration')

        bs2.client.env.juju_home = bs1.client.env.juju_home
        with bs2.existing_booted_context(upload_tools):
            ensure_able_to_migrate_model_between_controllers(
                bs1, bs2, upload_tools)

            with temp_dir() as temp:
                ensure_migrating_with_user_permissions(
                    bs1, bs2, upload_tools, temp)


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
    bundle = 'mongodb'
    application = 'mongodb'

    log.info('Deploying charm')
    # Don't move the default model so we can reuse it in later tests.
    test_model = source_environ.client.add_model(
        source_environ.client.env.clone('example-model'))
    test_model.juju("deploy", (bundle))
    test_model.wait_for_started()
    test_model.wait_for_workloads()
    test_deployed_mongo_is_up(test_model)

    log.info('Initiating migration process')

    migration_target_client = migrate_model_to_controller(
        test_model, dest_environ.client)

    test_deployed_mongo_is_up(migration_target_client)
    ensure_model_is_functional(migration_target_client, application)


def migrate_model_to_controller(source_client, dest_client):
    source_client.controller_juju(
        'migrate',
        (source_client.env.environment,
         dest_client.env.controller.name))

    migration_target_client = dest_client.clone(
        dest_client.env.clone(
            source_client.env.environment))

    wait_for_model(
        migration_target_client, source_client.env.environment)

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


def ensure_migrating_with_user_permissions(
        source_bs, dest_bs, upload_tools, temp_dir):
    """Ensure migration fails when a user does not have the right permissions.

    A non-superuser on a controller cannot migrate their models between
    controllers.

    """
    # Create a user for both controllers that only has addmodel
    # permissions not superuser.
    new_user_home = os.path.join(temp_dir, 'example_user')
    os.makedirs(new_user_home)
    new_user = User('testuser', 'write', [])
    normal_user_client_1 = source_bs.client.register_user(
        new_user, new_user_home)
    source_bs.client.grant(new_user.name, 'addmodel')

    second_controller_name = '{}_controllerb'.format(new_user.name)
    normal_user_client_2 = dest_bs.client.register_user(
        new_user,
        new_user_home,
        controller_name=second_controller_name)
    dest_bs.client.grant(new_user.name, 'addmodel')

    user_new_model_client = normal_user_client_1.add_model(
        normal_user_client_1.env.clone('model-a'))

    charm_path = local_charm_path(
        charm='dummy-source', juju_ver=user_new_model_client.version)
    user_new_model_client.deploy(charm_path)
    user_new_model_client.wait_for_started()

    log.info('Attempting migration process')

    expect_migration_attempt_to_fail(
        user_new_model_client,
        normal_user_client_2)


def expect_migration_attempt_to_fail(source_client, dest_client):
    """Ensure that the migration attempt fails due to permissions.

    As we're capturing the stderr output it after we're done with it so it
    appears in test logs.

    """
    try:
        args = ['-c', source_client.env.controller.name,
                source_client.env.environment,
                dest_client.env.controller.name]
        log_output = source_client.get_juju_output(
            'migrate', *args, merge_stderr=True, include_e=False)
    except CalledProcessError as e:
        print(e.output, file=sys.stderr)
        if 'permission denied' not in e.output:
            raise
        log.info('SUCCESS: Migrate command failed as expected.')
    else:
        print(log_output, file=sys.stderr)
        raise JujuAssertionError('Migration did not fail as expected.')


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    bs1, bs2 = get_bootstrap_managers(args)

    assess_model_migration(bs1, bs2, args.upload_tools)

    return 0


if __name__ == '__main__':
    sys.exit(main())
