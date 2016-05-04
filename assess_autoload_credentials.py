#!/usr/bin/env python
"""Tests for the autoload-credentials command."""

from __future__ import print_function

import argparse
import contextlib
import fixtures
import logging
import os
import pexpect
import sys
import yaml

from utility import configure_logging, enforce_juju_path


__metaclass__ = type


log = logging.getLogger("assess_autoload_credentials")


@contextlib.contextmanager
def XDGDataPath():
    """Create a temp dir to act as XDG_DATA_HOME and update the env to use it.

    """
    with fixtures.TempHomeDir():
        with fixtures.TempDir() as tmpdir:
            with fixtures.EnvironmentVariable('XDG_DATA_HOME', tmpdir.path):
                yield tmpdir.path


@contextlib.contextmanager
def AWSEnvironment(user, access_key, secret_key):
    """Setup environment variables for AWS token details.

    :param user: Username to use for $USER
    :param access_key: access_key string
    :param secret_key: secret_key string

    """
    with contextlib.nested(
        fixtures.EnvironmentVariable('USER', user),
        fixtures.EnvironmentVariable('AWS_ACCESS_KEY_ID', access_key),
        fixtures.EnvironmentVariable('AWS_SECRET_ACCESS_KEY', secret_key)
    ):
        yield


def assess_autoload_credentials(juju_bin):

    test_autoload_credentials_stores_details(juju_bin)

    test_autoload_credentials_updates_existing(juju_bin)


def run_autoload_credentials(juju_bin):
    """Execute the command 'juju autoload-credentials'.

    Simple interaction, calls juju autoload-credentials selects the first
    option and then quits.

    :param juju_bin: the juju binary to use.

    """
    cmd = '{juju} autoload-credentials'.format(juju=juju_bin)
    process = pexpect.spawn(cmd)
    menu_strings = [
        '\r\n',
        'Looking for cloud and credential information locally...',
        '\r\n',
        '1. aws credential "testing" (new)',
        'Save any? Type number, or Q to quit, then enter.',
    ]

    process.expect(menu_strings)
    process.sendline('1')

    'Looking for cloud and credential information locally...\r\n'
    process.expect([
        '\r\n',
        'Enter cloud to which the credential belongs, or Q to quit [aws]'
    ])
    process.sendline()
    process.expect('Saved aws credential "testing" to cloud aws')
    process.sendline('Q')
    process.expect(pexpect.EOF)

    if process.isalive():
        print(str(process))
        raise AssertionError('juju process failed to terminate')


def test_autoload_credentials_stores_details(juju_bin):
    user = 'testing'
    access_key = 'xxx'
    secret_key = 'xxx'

    expected_details = {
        'auth-type': 'access-key',
        'access-key': access_key,
        'secret-key': secret_key
    }

    with XDGDataPath() as xdg_path:
        with AWSEnvironment(user, access_key, secret_key):
            run_autoload_credentials(juju_bin)

            credentials_file = os.path.join(
                xdg_path, 'juju', 'credentials.yaml'
            )
            assert_file_exists(credentials_file)
            assert_credential_file_contains_expected_results(
                credentials_file,
                user,
                expected_details
            )


def assert_credential_file_contains_expected_results(filepath, user, expected):
    with open(filepath, 'r') as f:
        yaml_contents = yaml.safe_load(f)
    details = yaml_contents['credentials']['aws'][user]

    for content in expected.keys():
        if expected[content] != details[content]:
            raise ValueError(
                'Expected {} but have {} for key: {}'.format(
                    expected[content],
                    details[content],
                    content
                )
            )


def assert_file_exists(file_path):
    if not os.path.exists(file_path):
        raise ValueError('File {} does not exists'.format(file_path))


def test_autoload_credentials_updates_existing(juju_bin):
    pass


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
