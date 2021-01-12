#!/usr/bin/env python3
"""Tests for the autoload-credentials command."""

from __future__ import print_function

import argparse
from collections import (
    defaultdict,
    namedtuple,
    )
from contextlib import contextmanager
import glob
import itertools
import json
import logging
import os
import sys
import tempfile
import re
from textwrap import dedent

import pexpect

from deploy_stack import BootstrapManager
from jujupy import (
    client_from_config,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    ensure_dir,
    scoped_environ,
    temp_dir,
    )


__metaclass__ = type


log = logging.getLogger("assess_autoload_credentials")


# Store details for querying the interactive command.
# cloud_listing: String response for choosing credential to save
# save_name: String response in which to save the credential under.
ExpectAnswers = namedtuple('ExpectAnswers', ['cloud_listing', 'save_name'])

# Store details for setting up a clouds credentials as well as what to compare
# during test.
# env_var_changes: dict
# expected_details: dict
# expect_answers: ExpectAnswers object
CloudDetails = namedtuple(
    'CloudDetails',
    ['env_var_changes', 'expected_details', 'expect_answers']
    )


class CredentialIdCounter:
    _counter = defaultdict(itertools.count)

    @classmethod
    def id(cls, provider_name):
        return cls._counter[provider_name].next()


def assess_autoload_credentials(args):
    test_scenarios = {
        'ec2': [('AWS using environment variables', aws_envvar_test_details),
                ('AWS using credentials file', aws_directory_test_details)],
        'openstack':
            [('OS using environment variables', openstack_envvar_test_details),
             ('OS using credentials file', openstack_directory_test_details)],
        'gce': [('GCE using envvar with credentials file',
                 gce_envvar_with_file_test_details),
                ('GCE using credentials file',
                 gce_file_test_details)],
        }

    client = client_from_config(args.env, args.juju_bin, False)
    client.env.load_yaml()
    provider = client.env.provider

    for scenario_name, scenario_setup in test_scenarios[provider]:
        log.info('* Starting test scenario: {}'.format(scenario_name))
        ensure_autoload_credentials_stores_details(client, scenario_setup)

    for scenario_name, scenario_setup in test_scenarios[provider]:
        log.info(
            '* Starting [overwrite] test, scenario: {}'.format(scenario_name))
        ensure_autoload_credentials_overwrite_existing(
            client, scenario_setup)

    real_credential_details = client_credentials_to_details(client)
    bs_manager = BootstrapManager.from_args(args)
    autoload_and_bootstrap(bs_manager, args.upload_tools,
                           real_credential_details, scenario_setup)


def client_credentials_to_details(client):
    """Convert the credentials in the client to details."""
    provider = client.env.provider
    log.info("provider: {}".format(provider))
    cloud_name = client.env.get_cloud()
    log.info("cloud_name: {}".format(cloud_name))
    credentials = client.env.get_cloud_credentials()
    if 'ec2' == provider:
        return {'secret_key': credentials['secret-key'],
                'access_key': credentials['access-key'],
                }
    if 'gce' == provider:
        gce_prepare_for_load()
        return {'client_id': credentials['client-id'],
                'client_email': credentials['client-email'],
                'private_key': credentials['private-key'],
                }
    if 'openstack' == provider:
        os_cloud = client.env.clouds['clouds'][cloud_name]
        return {'os_tenant_name': credentials['tenant-name'],
                'os_password': credentials['password'],
                'os_region_name': client.env.get_region(),
                'os_auth_url': os_cloud['endpoint'],
                }


@contextmanager
def fake_juju_data():
    with scoped_environ():
        with temp_dir() as temp:
            os.environ['JUJU_HOME'] = temp
            os.environ['JUJU_DATA'] = temp
            yield


@contextmanager
def begin_autoload_test(client_base):
    client = client_base.clone(env=client_base.env.clone())
    with temp_dir() as tmp_dir:
        tmp_juju_home = tempfile.mkdtemp(dir=tmp_dir)
        tmp_scratch_dir = tempfile.mkdtemp(dir=tmp_dir)
        client.env.juju_home = tmp_juju_home
        client.env.load_yaml()
        with fake_juju_data():
            yield client, tmp_scratch_dir


def ensure_autoload_credentials_stores_details(client_base, cloud_details_fn):
    """Test covering loading and storing credentials using autoload-credentials

    :param client: ModelClient object to use for the test run.
    :param cloud_details_fn: A callable that takes the 3 arguments `user`
      string, `tmp_dir` path string and client ModelClient and will returns a
      `CloudDetails` object used to setup creation of credential details &
      comparison of the result.

    """
    user = 'testing-user'
    with begin_autoload_test(client_base) as (client, tmp_scratch_dir):
        cloud_details = cloud_details_fn(user, tmp_scratch_dir, client)

        run_autoload_credentials(
            client,
            cloud_details.env_var_changes,
            cloud_details.expect_answers)

        client.env.load_yaml()

        assert_credentials_contains_expected_results(
            client.env.credentials,
            cloud_details.expected_details)


def ensure_autoload_credentials_overwrite_existing(client_base,
                                                   cloud_details_fn):
    """Storing credentials using autoload-credentials must overwrite existing.

    :param client: ModelClient object to use for the test run.
    :param cloud_details_fn: A callable that takes the 3 arguments `user`
      string, `tmp_dir` path string and client ModelClient and will returns a
      `CloudDetails` object used to setup creation of credential details &
      comparison of the result.

    """
    user = 'testing-user'
    with begin_autoload_test(client_base) as (client, tmp_scratch_dir):
        initial_details = cloud_details_fn(
            user, tmp_scratch_dir, client)

        run_autoload_credentials(
            client,
            initial_details.env_var_changes,
            initial_details.expect_answers)

        # Now run again with a second lot of details.
        overwrite_details = cloud_details_fn(user, tmp_scratch_dir, client)

        if (
                overwrite_details.expected_details ==
                initial_details.expected_details):
            raise ValueError(
                'Attempting to use identical values for overwriting')

        run_autoload_credentials(
            client,
            overwrite_details.env_var_changes,
            overwrite_details.expect_answers)

        client.env.load_yaml()

        assert_credentials_contains_expected_results(
            client.env.credentials,
            overwrite_details.expected_details)


def autoload_and_bootstrap(bs_manager, upload_tools, real_credentials,
                           cloud_details_fn):
    """Ensure we can bootstrap after autoloading credentials."""
    with begin_autoload_test(bs_manager.client) as (client_na,
                                                    tmp_scratch_dir):
        # Do not overwrite real JUJU_DATA/JUJU_HOME/cloud-city dir.
        bs_manager.client.env.juju_home = client_na.env.juju_home
        bs_manager.tear_down_client.env.juju_home = client_na.env.juju_home
        # Openstack needs the real username.
        user = client_na.env.get_option('username', 'testing-user')
        cloud_details = cloud_details_fn(
            user, tmp_scratch_dir, bs_manager.client, real_credentials)
        # Reset the client's credentials before autoload.
        bs_manager.client.env.credentials = {}

        with bs_manager.top_context() as machines:
            with bs_manager.bootstrap_context(
                    machines,
                    omit_config=bs_manager.client.bootstrap_replaces):
                run_autoload_credentials(
                    bs_manager.client,
                    cloud_details.env_var_changes,
                    cloud_details.expect_answers)
                bs_manager.client.env.load_yaml()

                bs_manager.client.bootstrap(
                    upload_tools=upload_tools,
                    bootstrap_series=bs_manager.series,
                    credential=user)
                bs_manager.client.kill_controller()


def assert_credentials_contains_expected_results(credentials, expected):
    if credentials != expected:
        raise ValueError(
            'Actual credentials do not match expected credentials.\n'
            'Expected: {expected}\nGot: {got}\n'.format(
                expected=expected,
                got=credentials))
    log.info('PASS: credentials == expected')


def run_autoload_credentials(client, envvars, answers):
    """Execute the command 'juju autoload-credentials'.

    Simple interaction, calls juju autoload-credentials selects the first
    option and then quits.

    :param client: ModelClient from which juju will be called.
    :param envvars: Dictionary containing environment variables to be used
      during execution.
    :param answers: ExpectAnswers object containing answers for the interactive
      command

    """

    process = client.expect(
        'autoload-credentials', extra_env=envvars, include_e=False)
    selection = 1
    while not process.eof():
        out = process.readline()
        pattern = '.*(\d). {} \(.*\).*'.format(answers.cloud_listing)
        match = re.match(pattern, out)
        if match:
            selection = match.group(1)
            break
    process.sendline(selection)

    process.expect(
        '(Select the cloud it belongs to|Enter cloud to which the credential)'
        '.* Q to quit.*')
    process.sendline(answers.save_name)
    process.expect(
        'Saved {listing_display} to cloud {save_name}'.format(
            listing_display=answers.cloud_listing,
            save_name=answers.save_name))
    process.sendline('q')
    process.expect(pexpect.EOF)

    if process.isalive():
        log.debug('juju process is still running: {}'.format(str(process)))
        process.terminate(force=True)
        raise AssertionError('juju process failed to terminate')


def aws_envvar_test_details(user, tmp_dir, client, credential_details=None):
    """client is un-used for AWS"""
    credential_details = credential_details or aws_credential_dict_generator()
    access_key = credential_details['access_key']
    secret_key = credential_details['secret_key']
    env_var_changes = get_aws_environment(user, access_key, secret_key)

    answers = ExpectAnswers(
        cloud_listing='aws credential "{}"'.format(user),
        save_name='aws')

    expected_details = get_aws_expected_details_dict(
        user, access_key, secret_key)

    return CloudDetails(env_var_changes, expected_details, answers)


def aws_directory_test_details(user, tmp_dir, client, credential_details=None):
    """client is un-used for AWS"""
    credential_details = credential_details or aws_credential_dict_generator()
    access_key = credential_details['access_key']
    secret_key = credential_details['secret_key']
    expected_details = get_aws_expected_details_dict(
        user, access_key, secret_key)

    write_aws_config_file(user, tmp_dir, access_key, secret_key)

    answers = ExpectAnswers(
        cloud_listing='aws credential "{}"'.format(user),
        save_name='aws')

    env_var_changes = dict(HOME=tmp_dir)

    return CloudDetails(env_var_changes, expected_details, answers)


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
    """Return a dictionary containing keys suitable for AWS env vars."""
    return dict(
        USER=user,
        AWS_ACCESS_KEY_ID=access_key,
        AWS_SECRET_ACCESS_KEY=secret_key)


def write_aws_config_file(user, tmp_dir, access_key, secret_key):
    """Write aws credentials file to tmp_dir

    :return: String path of created credentials file.

    """
    config_dir = os.path.join(tmp_dir, '.aws')
    config_file = os.path.join(config_dir, 'credentials')
    ensure_dir(config_dir)

    config_contents = dedent("""\
    [{}]
    aws_access_key_id={}
    aws_secret_access_key={}
    """.format(user, access_key, secret_key))

    with open(config_file, 'w') as f:
        f.write(config_contents)

    return config_file


def aws_credential_dict_generator():
    call_id = CredentialIdCounter.id('aws')
    creds = 'aws-credentials-{}'.format(call_id)
    return dict(
        access_key=creds,
        secret_key=creds)


def openstack_envvar_test_details(
        user, tmp_dir, client, credential_details=None):
    if credential_details is None:
        region = client.env.get_region()
        log.info(
            'Generating credential_details for openstack {}'.format(region))
        credential_details = openstack_credential_dict_generator(region)

    expected_details, answers = setup_basic_openstack_test_details(
        client, user, credential_details)
    env_var_changes = get_openstack_envvar_changes(user, credential_details)
    return CloudDetails(env_var_changes, expected_details, answers)


def get_openstack_envvar_changes(user, credential_details):
    return dict(
        USER=user,
        OS_USERNAME=user,
        OS_PASSWORD=credential_details['os_password'],
        OS_TENANT_NAME=credential_details['os_tenant_name'],
        OS_AUTH_URL=credential_details['os_auth_url'],
        OS_REGION_NAME=credential_details['os_region_name'],
        )


def openstack_directory_test_details(user, tmp_dir, client,
                                     credential_details=None):
    if credential_details is None:
        region = client.env.get_region()
        log.info(
            'Generating credential_details for openstack {}'.format(region))
        credential_details = openstack_credential_dict_generator(region)

    expected_details, answers = setup_basic_openstack_test_details(
        client, user, credential_details)
    write_openstack_config_file(tmp_dir, user, credential_details)
    env_var_changes = dict(HOME=tmp_dir)

    return CloudDetails(env_var_changes, expected_details, answers)


def setup_basic_openstack_test_details(client, user, credential_details):
    ensure_openstack_personal_cloud_exists(client)
    expected_details = get_openstack_expected_details_dict(
        user, credential_details)
    answers = ExpectAnswers(
        cloud_listing='openstack region ".*" project "{}" user "{}"'.format(
            credential_details['os_tenant_name'],
            user),
        save_name='testing-openstack')

    return expected_details, answers


def write_openstack_config_file(tmp_dir, user, credential_details):
    credentials_file = os.path.join(tmp_dir, '.novarc')
    with open(credentials_file, 'w') as f:
        credentials = dedent("""\
        export OS_USERNAME={user}
        export OS_PASSWORD={password}
        export OS_TENANT_NAME={tenant_name}
        export OS_AUTH_URL={auth_url}
        export OS_REGION_NAME={region}
        """.format(
            user=user,
            password=credential_details['os_password'],
            tenant_name=credential_details['os_tenant_name'],
            auth_url=credential_details['os_auth_url'],
            region=credential_details['os_region_name'],
            ))
        f.write(credentials)
    return credentials_file


def ensure_openstack_personal_cloud_exists(client):
    juju_home = client.env.juju_home
    if not juju_home.startswith('/tmp'):
        raise ValueError('JUJU_HOME is wrongly set to: {}'.format(juju_home))
    if client.env.clouds['clouds']:
        cloud_name = client.env.get_cloud()
        regions = client.env.clouds['clouds'][cloud_name]['regions']
    else:
        regions = {'region1': {}}
    os_cloud = {
        'testing-openstack': {
            'type': 'openstack',
            'auth-types': ['userpass'],
            'endpoint': client.env.get_option('auth-url'),
            'regions': regions
            }
        }
    client.env.clouds['clouds'] = os_cloud
    client.env.dump_yaml(juju_home)


def get_openstack_expected_details_dict(user, credential_details):
    return {
        'credentials': {
            'testing-openstack': {
                'default-region': credential_details['os_region_name'],
                user: {
                    'auth-type': 'userpass',
                    'domain-name': '',
                    'password': credential_details['os_password'],
                    'tenant-name': credential_details['os_tenant_name'],
                    'username': user,
                    'user-domain-name': '',
                    'project-domain-name': ''
                    }
                }
            }
        }


def openstack_credential_dict_generator(region):
    call_id = CredentialIdCounter.id('openstack')
    creds = 'openstack-credentials-{}'.format(call_id)
    return dict(
        os_tenant_name=creds,
        os_password=creds,
        os_auth_url='https://keystone.example.com:443/v2.0/',
        os_region_name=region)


def gce_prepare_for_load():
    GCE_AC = 'GOOGLE_APPLICATION_CREDENTIALS'
    if GCE_AC not in os.environ:
        juju_home = os.environ['JUJU_HOME']
        file_names = glob.glob(os.path.join(juju_home, 'gce-*.json'))
        if 1 != len(file_names):
            raise RuntimeError('Found {} candidate goodle credential files,'
                               ' requires 1.'.format(len(file_names)))
        os.environ[GCE_AC] = file_names[0]
    log.info('{}={}'.format(GCE_AC, os.environ[GCE_AC]))


def gce_envvar_with_file_test_details(user, tmp_dir, client,
                                      credential_details=None):
    if credential_details is None:
        credential_details = gce_credential_dict_generator()
    credentials_path = write_gce_config_file(tmp_dir, credential_details)

    answers = ExpectAnswers(
        cloud_listing='google credential "{}"'.format(
            credential_details['client_email']),
        save_name='google')

    expected_details = get_gce_expected_details_dict(user, credentials_path)

    env_var_changes = dict(
        USER=user,
        GOOGLE_APPLICATION_CREDENTIALS=credentials_path,
        )

    return CloudDetails(env_var_changes, expected_details, answers)


def gce_file_test_details(user, tmp_dir, client, credential_details=None):
    if credential_details is None:
        credential_details = gce_credential_dict_generator()

    home_path, credentials_path = write_gce_home_config_file(
        tmp_dir, credential_details)

    answers = ExpectAnswers(
        cloud_listing='google credential "{}"'.format(
            credential_details['client_email']),
        save_name='google')

    expected_details = get_gce_expected_details_dict(user, credentials_path)

    env_var_changes = dict(USER=user, HOME=home_path)

    return CloudDetails(env_var_changes, expected_details, answers)


def write_gce_config_file(tmp_dir, credential_details, filename=None):

    details = dict(
        type='service_account',
        client_id=credential_details['client_id'],
        client_email=credential_details['client_email'],
        private_key=credential_details['private_key']
        )

    # Generate a unique filename if none provided as this is stored and used in
    # comparisons.
    filename = filename or 'gce-file-config-{}.json'.format(
        CredentialIdCounter.id('gce-fileconfig'))
    credential_file = os.path.join(tmp_dir, filename)
    with open(credential_file, 'w') as f:
        json.dump(details, f)

    return credential_file


def write_gce_home_config_file(tmp_dir, credential_details):
    """Returns a tuple contining a new HOME path and credential file path."""
    # Add a unique string for home dir so each file path is unique within the
    # stored credentials file.
    home_dir = os.path.join(tmp_dir, 'gce-homedir-{}'.format(
        CredentialIdCounter.id('gce-homedir')))
    credential_path = os.path.join(home_dir, '.config', 'gcloud')
    os.makedirs(credential_path)

    written_credentials_path = write_gce_config_file(
        credential_path,
        credential_details,
        'application_default_credentials.json')

    return home_dir, written_credentials_path


def get_gce_expected_details_dict(user, credentials_path):
    return {
        'credentials': {
            'google': {
                user: {
                    'auth-type': 'jsonfile',
                    'file': credentials_path,
                    }
                }
            }
        }


def gce_credential_dict_generator():
    call_id = CredentialIdCounter.id('gce')
    creds = 'gce-credentials-{}'.format(call_id)
    return dict(
        client_id=creds,
        client_email='{}@example.com'.format(creds),
        private_key=creds
        )


def parse_args(argv):
    """Parse all arguments."""
    parser = argparse.ArgumentParser(
        description="Test autoload-credentials command.")
    add_basic_testing_arguments(parser, existing=False)
    return parser.parse_args(argv)


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)

    assess_autoload_credentials(args)
    return 0


if __name__ == '__main__':
    sys.exit(main())
