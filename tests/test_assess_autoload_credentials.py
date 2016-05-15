"""Tests for assess_autoload_credentials module."""

from argparse import Namespace
import ConfigParser
import logging
from mock import patch
import StringIO

import assess_autoload_credentials as aac
from tests import (
    TestCase,
    parse_error,
    )
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = aac.parse_args(['/bin/juju'])
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
        juju_bin = '/bin/juju'
        args = aac.parse_args([juju_bin])
        self.assertEqual(
            args,
            Namespace(juju_bin=juju_bin, verbose=logging.INFO))


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
        """Each call must return uniquie details each time."""
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
                        'default': {
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
        access_key = 'access_key'
        secret_key = 'secret_key'

        with temp_dir() as tmp_dir:
            credentials_file = aac.write_aws_config_file(
                tmp_dir, access_key, secret_key)
            credentials = ConfigParser.ConfigParser()
            with open(credentials_file, 'r') as f:
                credentials.readfp(f)

        expected_items = [
            ('aws_access_key_id', access_key),
            ('aws_secret_access_key', secret_key)]

        self.assertEqual(credentials.sections(), ['default'])
        self.assertEqual(
            credentials.items('default'), expected_items)


class TestOpenStackHelpers(TestCase):

    def test_credential_dict_generator_returns_different_details(self):
        """Each call must return uniquie details each time."""
        first_details = aac.openstack_credential_dict_generator()
        second_details = aac.openstack_credential_dict_generator()

        self.assertNotEqual(first_details, second_details)

    def test_expected_details_dict_returns_correct_values(self):
        user = 'username'
        os_password = 'password'
        os_tenant_name = 'tenant name'
        expected_details = aac.get_openstack_expected_details_dict(
            user, {
                'os_password': os_password,
                'os_tenant_name': os_tenant_name,
                })

        self.assertEqual(
            expected_details, {
                'credentials': {
                    'testing_openstack': {
                        user: {
                            'auth-type': 'userpass',
                            'domain-name': '',
                            'password': os_password,
                            'tenant-name': os_tenant_name,
                            'username': user
                            }
                        }
                    }
                })

    def test_get_openstack_envvar_changes_returns_correct_values(self):
        user = 'username'
        os_password = 'password'
        os_tenant_name = 'tenant name'
        env_var_changes = aac.get_openstack_envvar_changes(
            user, {
                'os_password': os_password,
                'os_tenant_name': os_tenant_name,
                })

        self.assertEqual(
            env_var_changes, {
                'USER': user,
                'OS_USERNAME': user,
                'OS_PASSWORD': os_password,
                'OS_TENANT_NAME': os_tenant_name,
                })


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
