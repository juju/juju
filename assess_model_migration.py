#!/usr/bin/env python
"""Tests for the Model Migration feature"""

from __future__ import print_function

import argparse
import logging
import os
from subprocess import CalledProcessError
import sys
from time import sleep

from deploy_stack import BootstrapManager
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    until_timeout,
)


__metaclass__ = type


log = logging.getLogger("assess_model_migration")


def assess_model_migration(bs1, bs2, upload_tools):
    ensure_able_to_migrate_model_between_controllers(bs1, bs2, upload_tools)


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
    raise AssertionError(
        'Model \'{}\' failed to appear after {} seconds'.format(
            model_name, timeout
        ))


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
    """Test simple migration of a model to another controller.

    Ensure that migration a model that has an application deployed upon it is
    able to continue it's operation after the migration process.

    """
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
                (bs1.client.env.environment, bs2.client.env.controller.name))

            migration_target_client = bs2.client.clone(
                bs2.client.env.clone(bs1.client.env.environment))

            wait_for_model(migration_target_client, bs1.client.env.environment)

            # For logging purposes
            migration_target_client.show_status()

            migration_target_client.wait_for_started()
            test_deployed_mongo_is_up(migration_target_client)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    bs1, bs2 = get_bootstrap_managers(args)

    assess_model_migration(bs1, bs2, args.upload_tools)

    return 0


if __name__ == '__main__':
    sys.exit(main())
