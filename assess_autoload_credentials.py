#!/usr/bin/env python
"""Tests for the autoload-credentials command."""

from __future__ import print_function

import argparse
import logging
import os
import pexpect
import sys

from textwrap import dedent

from jujupy import EnvJujuClient, JujuData
from utility import (
    configure_logging,
    enforce_juju_path,
    temp_dir,
    ensure_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_autoload_credentials")


def assess_autoload_credentials(juju_bin):
    test_scenarios = [
        ('AWS using environment variables', aws_envvar_test_details),
        ('AWS using credentials file', aws_directory_test_details),
    ]

    for scenario_name, scenario_setup in test_scenarios:
        log.info('* Starting test scenario: {}'.format(scenario_name))
        test_autoload_credentials_stores_details(juju_bin, scenario_setup)


def test_autoload_credentials_stores_details(juju_bin, cloud_details_fn):
    """Test covering loading and storing credentials using autoload-credentials

    :param juju_bin: The full path to the juju binary to use for the test run.
    :para cloud_details_fn: A callable that takes the argument 'user' and
      'tmp_dir' that returns a tuple of:
        (dict -> environment variable changse,
        dict -> expected credential details)
      used to setup creation of credential details & comparison of the result.

    """
    user = 'testing_user'
    with temp_dir() as tmp_dir:
        client = EnvJujuClient.by_version(
            JujuData('local', juju_home=tmp_dir), juju_bin, False)

        env_var_changes, expected_details = cloud_details_fn(user, tmp_dir)
        # Inject well known username.
        env_var_changes.update({'USER': user})

        run_autoload_credentials(client, env_var_changes)

        client.env.load_yaml()

        assert_credentials_contains_expected_results(
            client.env.credentials,
            expected_details)


def assert_credentials_contains_expected_results(credentials, expected):
    if credentials != expected:
        raise ValueError(
            'Actual credentials do not match expected credentials.\n'
            'Expected: {expected}\nGot: {got}\n'.format(
                expected=expected,
                got=credentials
            )
        )


def run_autoload_credentials(client, envvars):
    """Execute the command 'juju autoload-credentials'.

    Simple interaction, calls juju autoload-credentials selects the first
    option and then quits.

    :param client: EnvJujuClient from which juju will be called.
    :param envvars: Dictionary containing environment variables to be used
      during execution.
      Note. Must contain a value for QUESTION_CLOUD_NAME to match against.
      Note. Must contain a value for SAVE_CLOUD_NAME to save against.

    """
    # Get juju path from client as we need to use it interactively.
    process = client.expect(
        'autoload-credentials', extra_env=envvars, include_e=False)
    process.expect(
        '.*1. \w+ credential "{}" \(new\).*'.format(
            envvars['QUESTION_CLOUD_NAME']))
    process.sendline('1')

    process.expect(
        'Enter cloud to which the credential belongs, or Q to quit.*')
    process.sendline(envvars['SAVE_CLOUD_NAME'])
    process.expect(
        'Saved aws credential "{}" to cloud \w+'.format(
            envvars['QUESTION_CLOUD_NAME']))
    process.sendline('q')
    process.expect(pexpect.EOF)

    if process.isalive():
        print(str(process))
        raise AssertionError('juju process failed to terminate')


def aws_envvar_test_details(
        user, tmp_dir, access_key='access_key', secret_key='secret_key'):
    env_var_changes = get_aws_environment(user, access_key, secret_key)

    expected_details = get_aws_expected_details_dict(
        user, access_key, secret_key)

    return env_var_changes, expected_details


def aws_directory_test_details(
        user, tmp_dir, access_key='access_key', secret_key='secret_key'
):
    expected_details = get_aws_expected_details_dict(
        'default', access_key, secret_key)

    write_aws_config_file(tmp_dir, access_key, secret_key)

    env_var_changes = dict(
        HOME=tmp_dir,
        SAVE_CLOUD_NAME='aws',
        QUESTION_CLOUD_NAME='default'
    )

    return env_var_changes, expected_details


def get_aws_expected_details_dict(cloud_name, access_key, secret_key):
    # Build credentials yaml file-like datastructure.
    return {
        'credentials': {
            'aws': {
                cloud_name: {
                    'auth-type': 'access-key',
                    'access-key': access_key,
                    'secret-key': secret_key,
                }
            }
        }
    }


def get_aws_environment(user, access_key, secret_key):
    """Return a dictionary containing keys suitable for AWS env vars.

    """
    return dict(
        SAVE_CLOUD_NAME='aws',
        QUESTION_CLOUD_NAME=user,
        AWS_ACCESS_KEY_ID=access_key,
        AWS_SECRET_ACCESS_KEY=secret_key
    )


def write_aws_config_file(tmp_dir, access_key, secret_key):
    """Write aws credentials file to tmp_dir

    :return: String path of created credentials file.

    """
    config_dir = os.path.join(tmp_dir, '.aws')
    config_file = os.path.join(config_dir, 'credentials')
    ensure_dir(config_dir)

    config_contents = dedent("""\
    [default]
    aws_access_key_id={}
    aws_secret_access_key={}
    """.format(access_key, secret_key))

    with open(config_file, 'w') as f:
        f.write(config_contents)

    return config_file


def parse_args(argv):
    parser = argparse.ArgumentParser(description="Test autoload-credentials.")
    parser.add_argument(
        'juju_bin', action=enforce_juju_path,
        help='Full path to the Juju binary.')
    parser.add_argument(
        '--verbose', action='store_const',
        default=logging.INFO, const=logging.DEBUG,
        help='Verbose test harness output.')

    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    assess_autoload_credentials(args.juju_bin)
    return 0


if __name__ == '__main__':
    sys.exit(main())
