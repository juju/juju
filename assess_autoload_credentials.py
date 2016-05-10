#!/usr/bin/env python
"""Tests for the autoload-credentials command."""

from __future__ import print_function

import argparse
import logging
import os
import pexpect
import sys
import tempfile
import yaml

from collections import namedtuple
from uuid import uuid4

from jujupy import EnvJujuClient, JujuData
from utility import (
    configure_logging,
    enforce_juju_path,
    temp_dir,
    ensure_dir,
)


__metaclass__ = type


log = logging.getLogger("assess_autoload_credentials")


# Store details for setting up a clouds credentials as well as what to compare
# during test.
# env_var_changes: dict
# expected_details: dict
CloudDetails = namedtuple(
    'CloudDetails',
    ['env_var_changes', 'expected_details']
)


def uuid_str():
    """UUID string generated from a random uuid."""
    return str(uuid4())


def assess_autoload_credentials(juju_bin):
    test_scenarios = [
        ('AWS using environment variables', aws_envvar_test_details),
        ('AWS using credentials file', aws_directory_test_details),
        ('OS using environment variables file', openstack_envvar_test_details),
    ]

    for scenario_name, scenario_setup in test_scenarios:
        log.info(
            '* Starting [storage] test, scenario: {}'.format(scenario_name)
        )
        test_autoload_credentials_stores_details(juju_bin, scenario_setup)

    for scenario_name, scenario_setup in test_scenarios:
        log.info(
            '* Starting [overwrite] test, scenario: {}'.format(scenario_name)
        )
        test_autoload_credentials_overwrite_existing(juju_bin, scenario_setup)


def test_autoload_credentials_stores_details(juju_bin, cloud_details_fn):
    """Test covering loading and storing credentials using autoload-credentials

    :param juju_bin: The full path to the juju binary to use for the test run.
    :para cloud_details_fn: A callable that takes the 3 arguments `user`
      string, `tmp_dir` path string and client EnvJujuClient and will returns a
      `CloudDetails` object used to setup creation of credential details &
      comparison of the result.

    """
    user = 'testing_user'
    with temp_dir() as tmp_dir:
        client = EnvJujuClient.by_version(
            JujuData('local', juju_home=tmp_dir), juju_bin, False
        )

        cloud_details = cloud_details_fn(user, tmp_dir, client)
        # Inject well known username.
        cloud_details.env_var_changes.update({'USER': user})

        run_autoload_credentials(client, cloud_details.env_var_changes)

        client.env.load_yaml()

        assert_credentials_contains_expected_results(
            client.env.credentials,
            cloud_details.expected_details
        )


def test_autoload_credentials_overwrite_existing(juju_bin, cloud_details_fn):
    """Storing credentials using autoload-credentials must overwrite existing.

    :param juju_bin: The full path to the juju binary to use for the test run.
    :para cloud_details_fn: A callable that takes the 3 arguments `user`
      string, `tmp_dir` path string and client EnvJujuClient and will returns a
      `CloudDetails` object used to setup creation of credential details &
      comparison of the result.

    """
    user = 'testing_user'
    with temp_dir() as tmp_dir:
        client = EnvJujuClient.by_version(
            JujuData('local', juju_home=tmp_dir), juju_bin, False
        )

        first_pass_cloud_details = cloud_details_fn(
            user, tmp_dir, client
        )
        # Inject well known username.
        first_pass_cloud_details.env_var_changes.update({'USER': user})

        run_autoload_credentials(
            client, first_pass_cloud_details.env_var_changes
        )

        # Now run again with a second lot of details.
        overwrite_cloud_details = cloud_details_fn(
            user, tmp_dir, client
        )
        # Inject well known username.
        overwrite_cloud_details.env_var_changes.update({'USER': user})
        run_autoload_credentials(
            client, overwrite_cloud_details.env_var_changes
        )

        client.env.load_yaml()

        assert_credentials_contains_expected_results(
            client.env.credentials,
            overwrite_cloud_details.expected_details
        )


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
      Note. Must contain a value for QUESTION_CLOUD_NAME to match against.
      Note. Must contain a value for SAVE_CLOUD_NAME to save against.

    """
    process = client.expect(
        'autoload-credentials', extra_env=envvars, include_e=False
    )
    process.expect(
        '.*1. {} \(.*\).*'.format(
            envvars['QUESTION_CLOUD_NAME']
        )
    )
    process.sendline('1')

    process.expect(
        'Enter cloud to which the credential belongs, or Q to quit.*'
    )
    process.sendline(envvars['SAVE_CLOUD_NAME'])
    process.expect(
        'Saved {} to cloud {}'.format(
            envvars['QUESTION_CLOUD_NAME'],
            envvars['SAVE_CLOUD_NAME']
        )
    )
    process.sendline('q')
    process.expect(pexpect.EOF)

    if process.isalive():
        print(str(process))
        raise AssertionError('juju process failed to terminate')


def aws_envvar_test_details(user, tmp_dir, client, credential_details=None):
    """client is un-used for AWS"""
    credential_details = credential_details or aws_credential_dict_generator()
    access_key = credential_details['access_key']
    secret_key = credential_details['secret_key']
    env_var_changes = get_aws_environment(user, access_key, secret_key)

    expected_details = get_aws_expected_details_dict(
        user, access_key, secret_key
    )

    return CloudDetails(env_var_changes, expected_details)


def aws_directory_test_details(user, tmp_dir, client, credential_details=None):
    """client is un-used for AWS"""
    credential_details = credential_details or aws_credential_dict_generator()
    access_key = credential_details['access_key']
    secret_key = credential_details['secret_key']
    expected_details = get_aws_expected_details_dict(
        'default', access_key, secret_key
    )

    write_aws_config_file(tmp_dir, access_key, secret_key)

    env_var_changes = dict(
        HOME=tmp_dir,
        SAVE_CLOUD_NAME='aws',
        QUESTION_CLOUD_NAME='aws credential "{}"'.format('default')
    )

    return CloudDetails(env_var_changes, expected_details)


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
        QUESTION_CLOUD_NAME='aws credential "{}"'.format(user),
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

    with open(config_file, 'w') as f:
        f.writelines([
            '[default]\n',
            'aws_access_key_id={}\n'.format(access_key),
            'aws_secret_access_key={}\n'.format(secret_key)
        ])

    return config_file


def aws_credential_dict_generator():
    return dict(
        access_key=uuid_str(),
        secret_key=uuid_str()
    )


def openstack_envvar_test_details(
        user, tmp_dir, client, credential_details=None
):
    if credential_details is None:
        credential_details = openstack_credential_dict_generator()

    ensure_openstack_personal_cloud_exists(client)

    expected_details = get_openstack_expected_details_dict(
        user, credential_details
    )

    env_var_changes = get_openstack_envvar_changes(user, credential_details)
    return CloudDetails(env_var_changes, expected_details)


def get_openstack_envvar_changes(user, credential_details):
    question = 'openstack region ".*" project "{}" user "{}"'.format(
        credential_details['os_tenant_name'],
        user
    )

    return dict(
        SAVE_CLOUD_NAME='testing_openstack',
        QUESTION_CLOUD_NAME=question,
        OS_USERNAME=user,
        OS_PASSWORD=credential_details['os_password'],
        OS_TENANT_NAME=credential_details['os_tenant_name'],
    )


def ensure_openstack_personal_cloud_exists(client):
    additional_cloud_settings = {
        'clouds': {
            'testing_openstack': {
                'type': 'openstack',
                'regions': {
                    'test1': {
                        'endpoint': 'https://testing.com',
                        'auth-types': ['access-key', 'userpass']
                    }
                }
            }
        }
    }

    cloud_listing = client.get_juju_output(
        'list-clouds', admin=False, include_e=False
    )
    if 'local:testing_openstack' not in cloud_listing:
        log.info('Creating and adding new cloud.')
        with tempfile.NamedTemporaryFile() as new_cloud_file:
            yaml.dump(additional_cloud_settings, new_cloud_file)
            client.juju(
                'add-cloud',
                ('testing_openstack', new_cloud_file.name),
                include_e=False
            )


def get_openstack_expected_details_dict(user, credential_details):
    return {
        'credentials': {
            'testing_openstack': {
                user: {
                    'auth-type': 'userpass',
                    'domain-name': '',
                    'password': credential_details['os_password'],
                    'tenant-name': credential_details['os_tenant_name'],
                    'username': user
                }
            }
        }
    }


def openstack_credential_dict_generator():
    return dict(
        os_tenant_name=uuid_str(),
        os_password=uuid_str()
    )


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
