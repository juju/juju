#!/usr/bin/env python
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
import logging
import os
from subprocess import CalledProcessError
import sys

from deploy_stack import BootstrapManager
import pexpect
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
    # ensure_able_to_migrate_model_between_controllers(bs1, bs2, upload_tools)
    # ensure_fail_to_migrate_to_lower_version_controller(bs1, bs2, upload_tools)
    ensure_migrating_with_user_permissions(bs1, bs2, upload_tools)


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


def ensure_migrating_with_user_permissions(bs1, bs2, upload_tools):
    """To migrate a user must have controller admin privileges.

    A Regular user (just read access to a model) cannot migrate a model
    An Admin user (write access to a controller admin model) can migrate a
    model

    """
    with bs1.booted_context(upload_tools):
        bs1.client.enable_feature('migration')

        # Create a normal user.
        register_token = bs1.client.add_user(
            'model-creator', permissions='write')
        # Set password and logout.
        _set_user_password(bs1.client, 'admin', 'juju')
        bs1.client.juju('logout', (), include_e=False)

        # Have this normie register and login
        _register_user(
            bs1.client, register_token, 'normal_controller', 'juju')
        # And create a model && deploy

        # Workout the id pub rsa to pass in :
        # --config authorized-keys="ssh-rsa
        def _get_rsa_pub(home_dir):
            # Actually find the right file. Hard code for now.
            full_path = os.path.join(home_dir, 'ssh', 'juju_id_rsa.pub')
            with open(full_path, 'r') as f:
                return f.read().replace('\n', '')

        # Need to log user into the controller.
        normal_user_client = bs1.client.clone(bs1.client.env.clone())
        _log_user_in(normal_user_client, 'model-creator', 'juju')
        rsa_pub = _get_rsa_pub(normal_user_client.env.juju_home)
        normal_user_client.env.config['authorized-keys'] = rsa_pub
        normal_user_client.add_model(normal_user_client.env.clone('new-model'))

        # Comment for now for time.
        # normal_user_client.juju('deploy', ('ubuntu'))
        # normal_user_client.wait_for_started()

        # Normie logs out, thanks normie.
        normal_user_client.controller_juju('logout', ())

        # Log back in admin/sys user.
        _log_user_in(bs1.client, 'admin', 'juju')

        # Bootstrap new env
        log.info('Booting second instance')
        bs2.client.env.juju_home = bs1.client.env.juju_home
        with bs2.existing_booted_context(upload_tools):
            log.info('Initiating migration process')
            # Admin starts migration
            # This should be the new-model client
            bs1.client.controller_juju(
                'migrate',
                (normal_user_client.env.environment,
                 'local.{}'.format(bs2.client.env.controller.name)))

            migration_target_client = bs2.client.clone(
                bs2.client.env.clone(normal_user_client.env.environment))

            wait_for_model(
                migration_target_client, normal_user_client.env.environment)

            migration_target_client.juju('status', ())
            migration_target_client.wait_for_started()


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    bs1, bs2 = get_bootstrap_managers(args)

    assess_model_migration(bs1, bs2, args.upload_tools)

    return 0


if __name__ == '__main__':
    sys.exit(main())
