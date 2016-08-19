#!/usr/bin/env python
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
import logging
import os
from subprocess import CalledProcessError
import sys
from time import sleep

import pexpect
from assess_user_grant_revoke import User
from deploy_stack import (
    BootstrapManager,
    assess_upgrade,
)
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
    # ensure_able_to_migrate_model_between_controllers(bs1, bs2, upload_tools)
    # ensure_fail_to_migrate_to_lower_version_controller(bs1, bs2, upload_tools)
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
    admin_client.upgrade_controller(force_version=False)
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
    # I think this should be changed so that source_environ is a client
    # (JujuEnv)
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


def _set_user_password(client, user, password):
    try:
        command = client.expect(
            'change-user-password', (user), include_e=False)
        command.expect('password:')
        command.sendline(password)
        command.expect('type password again:')
        command.sendline(password)
        command.expect('Your password has been updated.')
        command.expect(pexpect.EOF)
        if command.isalive():
            raise AssertionError(
                'Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
            raise AssertionError(
                'Registering user failed: pexpect session timed out')


def _log_user_in(client, user, password):
    # Create a contextmanger that handles all the shit re: using an expect.
    try:
        command = client.expect(
            'login', (user, '-c', client.env.controller.name), include_e=False)
        command.expect('password:')
        command.sendline(password)
        command.expect(pexpect.EOF)
        if command.isalive():
            raise AssertionError(
                'Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
            raise AssertionError(
                'Registering user failed: pexpect session timed out')


def _register_user(client, register_token, controller, password):
    try:
        command = client.expect('register', (register_token), include_e=False)
        command.expect('(?i)name .*: ')
        command.sendline(controller)
        command.expect('(?i)password')
        command.sendline(password)
        command.expect('(?i)password')
        command.sendline(password)
        command.expect(pexpect.EOF)
        if command.isalive():
            raise AssertionError(
                'Registering user failed: pexpect session still alive')
    except pexpect.TIMEOUT:
            raise AssertionError(
                'Registering user failed: pexpect session timed out')


def ensure_migrating_with_user_permissions(bs1, bs2, upload_tools, temp_dir):
    """To migrate a user must have controller admin privileges.

    A non-superuser on a controller cannot migrate their models between
    controllers.

    """
    with bs1.booted_context(upload_tools):
        bs1.client.enable_feature('migration')

        # Bootstrap new env
        log.info('Booting second instance')
        bs2.client.env.juju_home = bs1.client.env.juju_home
        with bs2.existing_booted_context(upload_tools):
            # Create a normal user.
            import ipdb; ipdb.set_trace()
            new_user_home = os.path.join(temp_dir, 'example_user')
            os.makedirs(new_user_home)
            new_user = User('testuser', 'write', [])
            normal_user_client_1 = bs1.client.register_user(
                new_user, new_user_home)
            bs1.client.grant(new_user.name, 'addmodel')
            # bs1.client.juju(
            #     'grant',
            #     (username, 'addmodel', '-c', bs1.client.env.controller.name),
            #     include_e=False)

            normal_user_client_2 = bs2.client.register_user(
                new_user, new_user_home)
            bs1.client.grant(new_user.name, 'addmodel')

            # Needed?
            # _set_user_password(bs1.client, 'admin', 'juju')

            # Workout the id pub rsa to pass in :
            # --config authorized-keys="ssh-rsa
            def _get_rsa_pub(home_dir):
                full_path = os.path.join(home_dir, 'ssh', 'juju_id_rsa.pub')
                with open(full_path, 'r') as f:
                    return f.read().replace('\n', '')

            # rsa_pub = _get_rsa_pub(normal_user_client.env.juju_home)
            # normal_user_client.env.config['authorized-keys'] = rsa_pub

            # Step into this method.
            normal_user_client_1.add_model(
                normal_user_client_1.env.clone('model-a'))

            # Comment for now for time.
            # normal_user_client.juju('deploy', ('ubuntu'))
            # normal_user_client.wait_for_started()

            log.info('Attempting migration process')

            # This should fail.
            normal_user_client_1.controller_juju(
                'migrate',
                (normal_user_client_1.env.environment,
                 normal_user_client_2.env.controller.name))


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    bs1, bs2 = get_bootstrap_managers(args)

    assess_model_migration(bs1, bs2, args.upload_tools)

    return 0


if __name__ == '__main__':
    sys.exit(main())
