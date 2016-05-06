"""Tests for assess_autoload_credentials module."""

import logging
import StringIO
from mock import patch
from tests import TestCase, parse_error

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
        self.assertNotIn('TODO', fake_stdout.getvalue())

    def test_verbose_is_set_to_debug_when_passed(self):
        args = aac.parse_args(['/bin/juju', '--verbose'])
        self.assertEqual(logging.DEBUG, args.verbose)

    def test_verbose_defaults_to_INFO(self):
        args = aac.parse_args(['/bin/juju'])
        self.assertEqual(logging.INFO, args.verbose)


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

    def test_aws_test_details_returns_correct_expected_details(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        username = 'user'
        env, expected = aac.aws_test_details(username, access_key, secret_key)

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
