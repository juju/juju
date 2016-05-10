"""Tests for assess_autoload_credentials module."""

from argparse import Namespace
import logging
import StringIO
import ConfigParser
from mock import patch
from tests import TestCase, parse_error

from utility import temp_dir
import assess_autoload_credentials as aac


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
            Namespace(juju_bin=juju_bin, verbose=logging.INFO)
        )


class TestHelpers(TestCase):

    def test_get_aws_environment_supplies_all_keys(self):
        access_key = 'access_key'
        secret_key = 'secret_key'
        username = 'username'

        env = aac.get_aws_environment(username, access_key, secret_key)

        self.assertDictEqual(
            env,
            dict(
                QUESTION_CLOUD_NAME=username,
                SAVE_CLOUD_NAME='aws',
                AWS_ACCESS_KEY_ID=access_key,
                AWS_SECRET_ACCESS_KEY=secret_key
            )
        )

    def test_aws_envvar_test_details_returns_correct_expected_details(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        env, expected = aac.aws_envvar_test_details(
            username, 'tmp_dir', access_key, secret_key
        )

        self.assertDictEqual(
            expected,
            {
                'credentials': {
                    'aws': {
                        username: {
                            'auth-type': 'access-key',
                            'access-key': access_key,
                            'secret-key': secret_key,
                        }
                    }
                }
            }
        )

    def test_aws_envvar_test_details_returns_correct_envvar_settings(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        env, expected = aac.aws_envvar_test_details(
            username, 'tmp_dir', access_key, secret_key
        )

        self.assertDictEqual(
            env,
            dict(
                SAVE_CLOUD_NAME='aws',
                QUESTION_CLOUD_NAME=username,
                AWS_ACCESS_KEY_ID=access_key,
                AWS_SECRET_ACCESS_KEY=secret_key
            )
        )

    def test_aws_directory_test_details_returns_correct_expected_details(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        with patch.object(aac, 'write_aws_config_file'):
            env, expected = aac.aws_directory_test_details(
                username, 'tmp_dir', access_key, secret_key
            )

        self.assertDictEqual(
            expected,
            {
                'credentials': {
                    'aws': {
                        'default': {
                            'auth-type': 'access-key',
                            'access-key': access_key,
                            'secret-key': secret_key,
                        }
                    }
                }
            }
        )

    def test_aws_directory_test_details_returns_envvar_settings(self):
        with patch.object(aac, 'write_aws_config_file'):
            env, expected = aac.aws_directory_test_details(
                'username', 'tmp_dir', 'access_key', 'secret_key'
            )
        self.assertDictEqual(
            env,
            dict(
                HOME='tmp_dir',
                SAVE_CLOUD_NAME='aws',
                QUESTION_CLOUD_NAME='default'
            )
        )

    def test_write_aws_config_file_writes_credentials_file(self):
        """Ensure the file created contains the correct details."""
        access_key = 'access_key'
        secret_key = 'secret_key'

        with temp_dir() as tmp_dir:
            credentials_file = aac.write_aws_config_file(
                tmp_dir, access_key, secret_key
            )
            credentials = ConfigParser.ConfigParser()
            with open(credentials_file, 'r') as f:
                credentials.readfp(f)

        self.assertEqual(credentials.sections(), ['default'])
        self.assertEqual(
            credentials.items('default'),
            [
                ('aws_access_key_id', access_key),
                ('aws_secret_access_key', secret_key),
            ]
        )


class TestAssertCredentialsContainsExpectedResults(TestCase):

    def test_does_not_raise_when_credentials_match(self):
        cred_actual = dict(key='value')
        cred_expected = dict(key='value')

        aac.assert_credentials_contains_expected_results(
            cred_actual, cred_expected
        )

    def test_raises_when_credentials_do_not_match(self):
        cred_actual = dict(key='value')
        cred_expected = dict(key='value', another_key='extra')

        self.assertRaises(
            ValueError,
            aac.assert_credentials_contains_expected_results,
            cred_actual,
            cred_expected,
        )
