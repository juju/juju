#!/usr/bin/env python
"""Tests for the autoload-credentials command."""

from __future__ import print_function

import argparse
import logging
import pexpect
import sys

from jujupy import EnvJujuClient, JujuData
from utility import (
    configure_logging,
    enforce_juju_path,
    temp_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_autoload_credentials")


def assess_autoload_credentials(juju_bin):

    test_autoload_credentials_stores_details(juju_bin)

    test_autoload_credentials_updates_existing(juju_bin)


def test_autoload_credentials_stores_details(juju_bin):
    user = 'testing_user'
    with temp_dir() as tmp_dir:
        client = EnvJujuClient.by_version(
            JujuData('local', juju_home=tmp_dir), juju_bin, False
        )

        env_var_changes, expected_details = aws_test_details(user=user)
        # Inject well known username.
        env_var_changes.update({'USER': user})

        run_autoload_credentials(client, env_var_changes)

        client.env.load_yaml()

        assert_credentials_contains_expected_results(
            client.env.credentials,
            expected_details
        )


def test_autoload_credentials_updates_existing(juju_bin):
    pass


def assert_credentials_contains_expected_results(
        credentials, expected_credentials
):
    if credentials != expected_credentials:
        raise ValueError(
            'Actual credentials do not match expected credentials.\n'
            'Expected: {expected}\nGot: {got}\n'.format(
                expected=expected_credentials,
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
      Note. Must contain a value for USER.

    """
    # Get juju path from client as we need to use it interactively.
    process = client.expect(
        'autoload-credentials', extra_env=envvars, include_e=False
    )
    process.expect(
        '.*1. aws credential "{}" \(new\).*'.format(envvars['USER'])
    )
    process.sendline('1')

    process.expect(
        'Enter cloud to which the credential belongs, or Q to quit \[aws\]'
    )
    process.sendline()
    process.expect(
        'Saved aws credential "{}" to cloud aws'.format(envvars['USER'])
    )
    process.sendline('q')
    process.expect(pexpect.EOF)

    if process.isalive():
        print(str(process))
        raise AssertionError('juju process failed to terminate')


def aws_test_details(user, access_key='access_key', secret_key='secret_key'):
    env_var_changes = get_aws_environment(access_key, secret_key)

    # Build credentials yaml file-like datastructure.
    expected_details = {
        'credentials': {
            'aws': {
                user: {
                    'auth-type': 'access-key',
                    'access-key': access_key,
                    'secret-key': secret_key,
                }
            }
        }
    }

    return env_var_changes, expected_details


def get_aws_environment(access_key, secret_key):
    """Return a dictionary containing keys suitable for AWS env vars.

    """
    return dict(AWS_ACCESS_KEY_ID=access_key, AWS_SECRET_ACCESS_KEY=secret_key)


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
