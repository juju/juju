"""Tests for assess_autoload_credentials module."""

from argparse import Namespace
import ConfigParser
import logging
from mock import patch
import os
import StringIO
from textwrap import dedent

import yaml

import assess_autoload_credentials as aac
from deploy_stack import BootstrapManager
from jujupy import fake_juju_client
from tests import (
    TestCase,
    parse_error,
    )
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = aac.parse_args(['env', '/bin/juju'])
        self.assertEqual('env', args.env)
        self.assertEqual('/bin/juju', args.juju_bin)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch('sys.stdout', fake_stdout):
                aac.parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertIn(
            'Test autoload-credentials command.', fake_stdout.getvalue())

    def test_verbose_is_set_to_debug_when_passed_verbose(self):
        args = aac.parse_args(['/bin/juju', '--verbose'])
        self.assertEqual(logging.DEBUG, args.verbose)

    def test_verbose_default_values(self):
        env = 'env'
        juju_bin = '/bin/juju'
        temp_env_name = 'functional-autoload-credentials'
        with temp_dir() as log:
            args = aac.parse_args([env, juju_bin, log, temp_env_name])
        self.assertEqual(
            args,
            Namespace(agent_stream=None, agent_url=None, bootstrap_host=None,
                      debug=False, deadline=None, env='env',
                      juju_bin='/bin/juju', keep_env=False, logs=log,
                      machine=[], region=None, series=None, to=None,
                      temp_env_name='functional-autoload-credentials',
                      upload_tools=False, verbose=logging.INFO, existing=None
                      ))


class TestCredentialIdCounter(TestCase):

    def setUp(self):
        # Make sure CredentialIdCounter is reset to initial values
        aac.CredentialIdCounter._counter.clear()

    def test_returns_zero_for_new_id(self):
        self.assertEqual(aac.CredentialIdCounter.id('test'), 0)

    def test_returns_iterations_for_same_id(self):
        generated_ids = [
            aac.CredentialIdCounter.id('test') for x in xrange(3)
        ]
        self.assertEqual(generated_ids, [0, 1, 2])

    def test_returns_new_ids_for_multiple_names(self):
        self.assertEqual(aac.CredentialIdCounter.id('test'), 0)
        self.assertEqual(aac.CredentialIdCounter.id('another_test'), 0)
        self.assertEqual(aac.CredentialIdCounter.id('test'), 1)
        self.assertEqual(aac.CredentialIdCounter.id('another_test'), 1)
        self.assertEqual(aac.CredentialIdCounter.id('test'), 2)


class TestAWSHelpers(TestCase):

    def test_credential_dict_generator_returns_different_details(self):
        """Each call must return unique details each time."""
        first_details = aac.aws_credential_dict_generator()
        second_details = aac.aws_credential_dict_generator()

        self.assertNotEqual(first_details, second_details)

    def test_get_aws_environment_supplies_all_keys(self):
        access_key = 'access_key'
        secret_key = 'secret_key'
        username = 'username'

        env = aac.get_aws_environment(username, access_key, secret_key)

        self.assertDictEqual(
            env,
            dict(
                USER=username,
                AWS_ACCESS_KEY_ID=access_key,
                AWS_SECRET_ACCESS_KEY=secret_key))

    def test_aws_envvar_test_details_returns_correct_expected_details(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        cloud_details = aac.aws_envvar_test_details(
            username,
            'tmp_dir',
            client=None,
            credential_details={
                'access_key': access_key,
                'secret_key': secret_key})

        self.assertDictEqual(
            cloud_details.expected_details, {
                'credentials': {
                    'aws': {
                        username: {
                            'auth-type': 'access-key',
                            'access-key': access_key,
                            'secret-key': secret_key,
                            }
                        }
                    }
                })

    def test_aws_envvar_test_details_returns_correct_envvar_settings(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        cloud_details = aac.aws_envvar_test_details(
            username,
            'tmp_dir',
            client=None,
            credential_details={
                'access_key': access_key,
                'secret_key': secret_key})

        self.assertDictEqual(
            cloud_details.env_var_changes,
            dict(
                USER=username,
                AWS_ACCESS_KEY_ID=access_key,
                AWS_SECRET_ACCESS_KEY=secret_key))

    def test_aws_directory_test_details_returns_correct_expected_details(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        with patch.object(aac, 'write_aws_config_file'):
            cloud_details = aac.aws_directory_test_details(
                username,
                'tmp_dir',
                client=None,
                credential_details={
                    'access_key': access_key, 'secret_key': secret_key})

        self.assertDictEqual(
            cloud_details.expected_details, {
                'credentials': {
                    'aws': {
                        username: {
                            'auth-type': 'access-key',
                            'access-key': access_key,
                            'secret-key': secret_key,
                            }
                        }
                    }
                })

    def test_aws_directory_test_details_returns_envvar_settings(self):
        with patch.object(aac, 'write_aws_config_file'):
            cloud_details = aac.aws_directory_test_details(
                'username',
                'tmp_dir',
                client=None)
        self.assertDictEqual(
            cloud_details.env_var_changes,
            dict(HOME='tmp_dir'))

    def test_write_aws_config_file_writes_credentials_file(self):
        """Ensure the file created contains the correct details."""
        user = 'different-user'
        access_key = 'access_key'
        secret_key = 'secret_key'

        with temp_dir() as tmp_dir:
            credentials_file = aac.write_aws_config_file(
                user, tmp_dir, access_key, secret_key)
            credentials = ConfigParser.ConfigParser()
            with open(credentials_file, 'r') as f:
                credentials.readfp(f)

        expected_items = [
            ('aws_access_key_id', access_key),
            ('aws_secret_access_key', secret_key)]

        self.assertEqual(credentials.sections(), [user])
        self.assertEqual(
            credentials.items(user), expected_items)


class TestOpenStackHelpers(TestCase):

    def test_credential_dict_generator_returns_different_details(self):
        """Each call must return uniquie details each time."""
        first_details = aac.openstack_credential_dict_generator('region1')
        second_details = aac.openstack_credential_dict_generator('region1')

        self.assertNotEqual(first_details, second_details)

    def test_expected_details_dict_returns_correct_values(self):
        os_username = 'username'
        os_password = 'password'
        os_tenant_name = 'tenant name'
        os_auth_url = 'url',
        os_region_name = 'region'
        expected_details = aac.get_openstack_expected_details_dict(
            os_username, {
                'os_password': os_password,
                'os_tenant_name': os_tenant_name,
                'os_auth_url': os_auth_url,
                'os_region_name': os_region_name,
                'os_user_name': os_username,
                })
        self.assertEqual(
            expected_details, {
                'credentials': {
                    'testing-openstack': {
                        'default-region': 'region',
                        os_username: {
                            'auth-type': 'userpass',
                            'domain-name': '',
                            'project-domain-name': '',
                            'user-domain-name': '',
                            'password': os_password,
                            'tenant-name': os_tenant_name,
                            'username': os_username
                            }
                        }
                    }
                })

    def test_get_openstack_envvar_changes_returns_correct_values(self):
        user = 'username'
        os_password = 'password'
        os_tenant_name = 'tenant name'
        os_auth_url = 'url',
        os_region_name = 'region'
        env_var_changes = aac.get_openstack_envvar_changes(
            user, {
                'os_password': os_password,
                'os_tenant_name': os_tenant_name,
                'os_auth_url': os_auth_url,
                'os_region_name': os_region_name,
                })

        self.assertEqual(
            env_var_changes, {
                'USER': user,
                'OS_USERNAME': user,
                'OS_PASSWORD': os_password,
                'OS_TENANT_NAME': os_tenant_name,
                'OS_AUTH_URL': os_auth_url,
                'OS_REGION_NAME': os_region_name,
                })

    def test_write_openstack_config_file_writes_credentials_file(self):
        """Ensure the file created contains the correct details."""
        credential_details = dict(
            os_tenant_name='tenant_name',
            os_password='password',
            os_auth_url='url',
            os_region_name='region')
        user = 'username'

        with temp_dir() as tmp_dir:
            credentials_file = aac.write_openstack_config_file(
                tmp_dir, user, credential_details)
            with open(credentials_file, 'r') as f:
                credential_contents = f.read()

        expected = dedent("""\
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

        self.assertEqual(credential_contents, expected)


class TestGCEHelpers(TestCase):
    def test_get_gce_expected_details_dict_returns_correct_details(self):
        user = 'username'
        cred_path = '/some/path'
        self.assertEqual(
            aac.get_gce_expected_details_dict(user, cred_path),
            {
                'credentials': {
                    'google': {
                        user: {
                            'auth-type': 'jsonfile',
                            'file': cred_path,
                            }
                        }
                    }
                })

    def test_gce_credential_dict_generator_returns_unique_details(self):
        self.assertNotEqual(
            aac.gce_credential_dict_generator(),
            aac.gce_credential_dict_generator())

    def test_write_gce_config_file_creates_unique_credential_file(self):
        credentials = dict(
            client_id='client_id',
            client_email='client_email',
            private_key='private_key',
            )

        with patch.object(aac.CredentialIdCounter, 'id') as id_gen:
            id_gen.return_value = 0
            with temp_dir() as tmp_dir:
                file_path = aac.write_gce_config_file(tmp_dir, credentials)
        self.assertEqual(
            file_path,
            os.path.join(tmp_dir, 'gce-file-config-{}.json'.format(0)))

    def test_write_gce_config_file_creates_named_credential_file(self):
        credentials = dict(
            client_id='client_id',
            client_email='client_email',
            private_key='private_key',
            )

        with temp_dir() as tmp_dir:
            file_path = aac.write_gce_config_file(
                tmp_dir, credentials, 'file_name')
        self.assertEqual(file_path, os.path.join(tmp_dir, 'file_name'))

    def test_credential_generator_returns_correct_formats(self):
        """Three items are needed, one of them must be an email address."""
        with patch.object(aac.CredentialIdCounter, 'id') as id_gen:
            id_gen.return_value = 0
            details = aac.gce_credential_dict_generator()

            self.assertEqual(details['private_key'], 'gce-credentials-0')
            self.assertEqual(details['client_id'], 'gce-credentials-0')
            self.assertEqual(
                details['client_email'], 'gce-credentials-0@example.com')


class TestAssertCredentialsContainsExpectedResults(TestCase):

    def test_does_not_raise_when_credentials_match(self):
        cred_actual = dict(key='value')
        cred_expected = dict(key='value')

        aac.assert_credentials_contains_expected_results(
            cred_actual, cred_expected)

    def test_raises_when_credentials_do_not_match(self):
        cred_actual = dict(key='value')
        cred_expected = dict(key='value', another_key='extra')

        self.assertRaises(
            ValueError,
            aac.assert_credentials_contains_expected_results,
            cred_actual,
            cred_expected)


def bogus_credentials():
    """return an client with unusable crednetials.

    It uses an openstack config to match the fake_juju.AutoloadCredentials.
    """
    client = fake_juju_client()
    client.env._config['type'] = 'openstack'
    client.env._config['auth-url'] = 'url'
    client.env._config['region'] = 'region'
    client.env.credentials = {
        'credentials': {'bogus': {}}}
    return client


class TestEnsureAutoloadCredentialsStoresDetails(TestCase):

    def test_existing_credentials_openstack(self):

            aac.ensure_autoload_credentials_stores_details(
                bogus_credentials(), aac.openstack_envvar_test_details)


class TestEnsureAutoloadCredentialsOverwriteExisting(TestCase):

    def test_overwrite_existing(self):
            aac.ensure_autoload_credentials_overwrite_existing(
                bogus_credentials(), aac.openstack_envvar_test_details)


class TestAutoloadAndBootstrap(TestCase):

    def test_autoload_and_bootstrap(self):

        def cloud_details_fn(user, tmp_dir, client, credential_details):
            return aac.CloudDetails(credential_details, None, None)

        client = fake_juju_client()
        upload_tools = False
        real_credential_details = {'cloud-username': 'user',
                                   'cloud-password': 'password'
                                   }
        credential_file_details = {'credentials': {'cloud': {
                                   'credentials': real_credential_details
                                   }}}

        def write_credentials(*args, **kwargs):
            file_name = os.path.join(client.env.juju_home, 'credentials.yaml')
            with open(file_name, 'w') as write_file:
                yaml.safe_dump(credential_file_details, write_file)

        def credential_check(*args, **kwargs):
            self.assertEqual(client.env.credentials, credential_file_details)

        with temp_dir() as log_dir:
            bs_manager = BootstrapManager(
                'env', client, client, None, [], None, None, None, None,
                log_dir, False, True, True)
            with patch('assess_autoload_credentials.run_autoload_credentials',
                       autospec=True,
                       side_effect=write_credentials) as run_autoload_mock:
                with patch.object(
                        bs_manager.client, 'bootstrap',
                        autospec=True,
                        side_effect=credential_check) as bootstrap_mock:
                    aac.autoload_and_bootstrap(bs_manager, upload_tools,
                                               real_credential_details,
                                               cloud_details_fn)
        run_autoload_mock.assert_called_once_with(
            client, real_credential_details, None)
        bootstrap_mock.assert_called_once_with(False, bootstrap_series=None,
                                               credential='testing-user')
