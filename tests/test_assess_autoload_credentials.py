"""Tests for assess_autoload_credentials module."""

import logging
import os
import StringIO
from mock import patch, Mock
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

        env = aac.get_aws_environment(access_key, secret_key)

        self.assertDictEqual(
            env,
            dict(
                AWS_ACCESS_KEY_ID=access_key,
                AWS_SECRET_ACCESS_KEY=secret_key
            )
        )

    def test_aws_test_details_returns_correct_expected_details(self):
        access_key = 'test_access_key'
        secret_key = 'test_secret_key'
        env, expected = aac.aws_test_details(access_key, secret_key)

        self.assertDictEqual(
            expected,
            {
                'auth-type': 'access-key',
                'access-key': access_key,
                'secret-key': secret_key
            }
        )

    def test_get_fake_environment_returns_populated_dict(self):
        user = 'test_user'
        tmp_dir = 'temporary/directory/path'
        env = aac.get_fake_environment(user, tmp_dir)

        self.assertDictEqual(env, dict(USER=user, XDG_DATA_HOME=tmp_dir))

    def test_get_juju_client_sets_juju_home_with_config(self):
        juju_path = '/path/to/juju'

        with temp_dir() as tmp_dir:
            with patch('jujupy.EnvJujuClient.by_version'):
                config, client = aac.get_juju_client(
                    juju_path, tmp_dir, Mock()
                )
                self.assertEqual(
                    config.juju_home,
                    os.path.join(tmp_dir, 'juju')
                )

    def test_get_juju_client_returns_EnvJujuClient(self):
        juju_path = '/path/to/juju'

        with temp_dir() as tmp_dir:
            with patch('jujupy.EnvJujuClient.by_version') as envclient_creator:
                config_object = Mock()
                config, client = aac.get_juju_client(
                    juju_path, tmp_dir, config_object
                )
                envclient_creator.assert_called_once_with(
                    config_object, juju_path, False
                )
