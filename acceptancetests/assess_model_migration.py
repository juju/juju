#!/usr/bin/env python3
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
from contextlib import contextmanager
import logging
import os
from subprocess import CalledProcessError
import sys
from time import sleep
import yaml

from remote import remote_from_address
from utility import (
    JujuAssertionError,
    add_basic_testing_arguments,
    configure_logging,
    qualified_model_name,
    temp_dir,
    until_timeout,
)
from assess_user_grant_revoke import User
from deploy_stack import (
    BootstrapManager,
    get_random_string
)
from jujupy.wait_condition import (
    ModelCheckFailed,
    wait_for_model_check,
)
from jujupy.workloads import (
    assert_deployed_charm_is_responding,
    deploy_dummy_source_to_new_model,
    deploy_simple_server_to_new_model,
)


__metaclass__ = type


log = logging.getLogger("assess_model_migration")


def assess_model_migration(bs1, bs2, args):
    with bs1.booted_context(args.upload_tools):
        bs2.client.env.juju_home = bs1.client.env.juju_home
        with bs2.existing_booted_context(args.upload_tools):
            source_client = bs2.client
            dest_client = bs1.client
            # Capture the migrated client so we can use it to assert it
            # continues to operate after the originating controller is torn
            # down.
            results = ensure_migration_with_resources_succeeds(
                source_client,
                dest_client)
            migrated_client, application, resource_contents = results

            ensure_model_logs_are_migrated(source_client, dest_client)
            ensure_api_login_redirects(source_client, dest_client)

        # Continue test where we ensure that a migrated model continues to
        # work after it's originating controller has been destroyed.
        assert_model_migrated_successfully(
            migrated_client, application, resource_contents)
        log.info(
            'SUCCESS: Model operational after origin controller destroyed')


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test model migration feature")
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def get_bootstrap_managers(args):
    """Create 2 bootstrap managers from the provided args.

    Need to make a couple of elements unique (e.g. environment name) so we can
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


def wait_until_model_disappears(client, model_name, timeout=600):
    """Waits for a while for 'model_name' model to no longer be listed.

    :raises JujuAssertionError: If the named model continues to be listed in
      list-models after specified timeout.
    """
    def model_check(client):
        try:
            models = client.get_controller_client().get_models()

            # 2.2-rc1 introduced new model listing output name/short-name.
            all_model_names = [
                m.get('short-name', m['name']) for m in models['models']]
            if model_name not in all_model_names:
                return True
        except CalledProcessError as e:
            # It's possible that we've tried to get status from the model as
            # it's being removed.
            # We can't consider the model gone yet until we don't get this
            # error and the model is no longer in the output.
            if 'cannot get model details' not in str(e.stderr):
                raise

    try:
        wait_for_model_check(client, model_check, timeout)
    except ModelCheckFailed:
        raise JujuAssertionError(
            'Model "{}" failed to be removed after {} seconds'.format(
                model_name, timeout))


def wait_for_model(client, model_name, timeout=600):
    """Wait for a given timeout for the client to see the model_name.

    :raises JujuAssertionError: If the named model does not appear in the
      specified timeout.
    """
    def model_check(client):
        models = client.get_controller_client().get_models()
        # 2.2-rc1 introduced new model listing output name/short-name.
        all_model_names = [
            m.get('short-name', m['name']) for m in models['models']]
        if model_name in all_model_names:
            return True
    try:
        wait_for_model_check(client, model_check, timeout)
    except ModelCheckFailed:
        raise JujuAssertionError(
            'Model "{}" failed to appear after {} seconds'.format(
                model_name, timeout))


def wait_for_migrating(client, timeout=600):
    """Block until provided model client has a migration status.

    :raises JujuAssertionError: If the status doesn't show migration within the
        `timeout` period.
    """
    model_name = client.env.environment
    with client.check_timeouts():
        with client.ignore_soft_deadline():
            for _ in until_timeout(timeout):
                model_details = client.show_model(model_name)
                migration_status = model_details[model_name]['status'].get(
                    'migration')
                if migration_status is not None:
                    return
                sleep(1)
            raise JujuAssertionError(
                'Model \'{}\' failed to start migration after'
                '{} seconds'.format(
                    model_name, timeout
                ))


def ensure_api_login_redirects(source_client, dest_client):
    """Login attempts must get transparently redirected to the new controller.
    """
    new_model_client = deploy_dummy_source_to_new_model(
        source_client, 'api-redirection')

    # show model controller details
    before_model_details = source_client.show_model()
    assert_model_has_correct_controller_uuid(source_client)

    log.info('Attempting migration process')

    migrated_model_client = migrate_model_to_controller(
        new_model_client, source_client, dest_client)

    # check show model controller details
    assert_model_has_correct_controller_uuid(migrated_model_client)

    after_migration_details = migrated_model_client.show_model()
    before_controller_uuid = before_model_details[
        source_client.env.environment]['controller-uuid']
    after_controller_uuid = after_migration_details[
        migrated_model_client.env.environment]['controller-uuid']
    if before_controller_uuid == after_controller_uuid:
        raise JujuAssertionError()

    # Check file for details.
    assert_data_file_lists_correct_controller_for_model(
        migrated_model_client,
        expected_controller=dest_client.env.controller.name)

    # Release the model machines back to the cloud since there are more tests
    # after this part.
    migrated_model_client.destroy_model()


def assert_data_file_lists_correct_controller_for_model(
        client, expected_controller):
    models_path = os.path.join(client.env.juju_home, 'models.yaml')
    with open(models_path, 'rt') as f:
        models_data = yaml.safe_load(f)

    controller_models = models_data[
        'controllers'][expected_controller]['models']

    model_name = '{}/{}'.format(client.env.user_name, client.env.environment)
    if model_name not in controller_models:
        raise JujuAssertionError()


def assert_model_has_correct_controller_uuid(client):
    model_details = client.show_model()
    model_controller_uuid = model_details[
        client.env.environment]['controller-uuid']
    controller_uuid = client.get_controller_uuid()
    if model_controller_uuid != controller_uuid:
        raise JujuAssertionError()


def ensure_migration_with_resources_succeeds(source_client, dest_client):
    """Test simple migration of a model to another controller.

    Ensure that migration a model that has an application, that uses resources,
    deployed upon it is able to continue it's operation after the migration
    process. This includes assertion that the resources are migrated correctly
    too.

    Given 2 bootstrapped environments:
      - Deploy an application with a resource
        - ensure it's operating as expected
      - Migrate that model to the other environment
        - Ensure it's operating as expected
        - Add a new unit to the application to ensure the model is functional

    :return: Tuple containing migrated client object and the resource string
      that the charm deployed to it outputs.

    """
    resource_contents = get_random_string()
    test_model, application = deploy_simple_server_to_new_model(
        source_client, 'example-model-resource', resource_contents)
    migration_target_client = migrate_model_to_controller(
        test_model, source_client, dest_client)
    assert_model_migrated_successfully(
        migration_target_client, application, resource_contents)

    log.info('SUCCESS: resources migrated')
    return migration_target_client, application, resource_contents


def assert_model_migrated_successfully(
        client, application, resource_contents=None):
    client.wait_for_workloads()
    assert_deployed_charm_is_responding(client, resource_contents)
    ensure_model_is_functional(client, application)

def migrate_model_to_controller(
        source_model, source_client, dest_client, include_user_name=False):
    log.info('Initiating migration process')
    model_name = get_full_model_name(source_model, include_user_name)

    migration_target_client = source_client.migrate(
        model_name, source_model.env.environment, source_model, dest_client,
        include_e=False,
        )

    try:
        wait_for_model(migration_target_client, source_model.env.environment)
        migration_target_client.wait_for_started()
        wait_until_model_disappears(
            source_model, source_model.env.environment, timeout=480)
    except JujuAssertionError as e:
        # Attempt to show model details as it might log migration failure
        # message.
        log.error(
            'Model failed to migrate. '
            'Attempting show-model for affected models.')
        try:
            source_model.juju('show-model', (model_name), include_e=False)
        except:  # noqa
            log.info('Ignoring failed output.')
            pass

        try:
            source_model.juju(
                'show-model',
                get_full_model_name(
                    migration_target_client, include_user_name),
                include_e=False)
        except:  # noqa
            log.info('Ignoring failed output.')
            pass
        raise e
    return migration_target_client


def get_full_model_name(client, include_user_name):
    # Construct model name based on rules of version + if username is needed.
    if include_user_name:
        return '{}:{}/{}'.format(
            client.env.controller.name,
            client.env.user_name,
            client.env.environment)

    return '{}:{}'.format(
        client.env.controller.name,
        client.env.environment)


def ensure_model_is_functional(client, application):
    """Ensures that the migrated model is functional

    Add unit to application to ensure the model is contactable and working.
    Ensure that added unit is created on a new machine (check for bug
    LP:1607599)
    """
    # Ensure model returns status before adding units
    client.get_status()
    client.juju('add-unit', (application,))
    client.wait_for_started()
    assert_units_on_different_machines(client, application)
    log.info('SUCCESS: migrated model is functional.')


def assert_units_on_different_machines(client, application):
    status = client.get_status()
    # Not all units will be machines (as we have subordinate apps.)
    unit_machines = [
        u[1]['machine'] for u in status.iter_units()
        if u[1].get('machine', None)]
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


def ensure_model_logs_are_migrated(source_client, dest_client, timeout=600):
    """Ensure logs are migrated when a model is migrated between controllers.

    :param source_client: ModelClient representing source controller to create
      model on and migrate that model from.
    :param dest_client: ModelClient for destination controller to migrate to.
    :param timeout: int seconds to wait for logs to appear in migrated model.
    """
    new_model_client = deploy_dummy_source_to_new_model(
        source_client, 'log-migration')
    before_migration_logs = new_model_client.get_juju_output(
        'debug-log', '--no-tail', '-l', 'DEBUG')
    log.info('Attempting migration process')
    migrated_model = migrate_model_to_controller(
        new_model_client, source_client, dest_client,
    )

    assert_logs_appear_in_client_model(
        migrated_model, before_migration_logs, timeout)
    # Destroy the model, on the more resource-constrained clouds we could do
    # with the machines going away.
    migrated_model.destroy_model()


def assert_logs_appear_in_client_model(client, expected_logs, timeout):
    """Assert that `expected_logs` appear in client logs within timeout.

    :param client: ModelClient object to query logs of.
    :param expected_logs: string containing log contents to check for.
    :param timeout: int seconds to wait for before raising JujuAssertionError.
    """
    for _ in until_timeout(timeout):
        current_logs = client.get_juju_output(
            'debug-log', '--no-tail', '--replay', '-l', 'DEBUG')
        if expected_logs in current_logs:
            log.info('SUCCESS: logs migrated.')
            return
        sleep(1)
    raise JujuAssertionError(
        'Logs failed to be migrated after {}'.format(timeout))


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs1, bs2 = get_bootstrap_managers(args)
    assess_model_migration(bs1, bs2, args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
