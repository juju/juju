#!/usr/bin/env python
"""Tests for the autoload-credentials command."""

from __future__ import print_function

import argparse
import logging
import os
import pexpect
import sys

from jujupy import EnvJujuClient, JujuData
from utility import (
    configure_logging,
    enforce_juju_path,
    ensure_dir,
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
        local_config, client = get_juju_client(
            juju_bin, tmp_dir, JujuData.from_config('local')
        )

        env_changes, expected_details = aws_test_details()

        extra_environment = get_fake_environment(user, tmp_dir)
        extra_environment.update(env_changes)
        run_autoload_credentials(client, extra_environment)

        local_config.load_yaml()

        assert_credential_file_contains_expected_results(
            local_config.credentials,
            user,
            expected_details
        )


def test_autoload_credentials_updates_existing(juju_bin):
    pass


def assert_credential_file_contains_expected_results(
        credentials, user, expected
):
    details = credentials['credentials']['aws'][user]

    for content in expected.keys():
        if expected[content] != details[content]:
            raise ValueError(
                'Expected {} but have {} for key: {}'.format(
                    expected[content],
                    details[content],
                    content
                )
            )


def get_juju_client(juju_bin, tmp_dir, config):
    juju_home = os.path.join(tmp_dir, 'juju')
    ensure_dir(juju_home)
    config.juju_home = juju_home
    client = EnvJujuClient.by_version(config, juju_bin, False)
    return config, client


def get_fake_environment(user, tmpdir):
    """Return a dictionary setting up a fake environment.

    :param user: Username to set $USER to
    :para tmpdir: Directory path to use for $XDG_DATA_HOME.

    """
    return dict(
        XDG_DATA_HOME=tmpdir,
        USER=user,
    )


def run_autoload_credentials(juju_client, envvars):
    """Execute the command 'juju autoload-credentials'.

    Simple interaction, calls juju autoload-credentials selects the first
    option and then quits.

    :param juju_client: A EnvJujuClient from which juju will be called.
    :param envvars: Dictionary containing environment variables to be used
      during execution.
      Note. Must contain a value for USER.

    """
    # Get juju path from client as we need to use it interactively.
    cmd = '{juju} autoload-credentials'.format(juju=juju_client.full_path)
    process = pexpect.spawn(cmd, env=envvars)
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


def aws_test_details(access_key='access_key', secret_key='secret_key'):
    env_changes = get_aws_environment(access_key, secret_key)

    expected_details = {
        'auth-type': 'access-key',
        'access-key': access_key,
        'secret-key': secret_key,
    }

    return env_changes, expected_details


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
