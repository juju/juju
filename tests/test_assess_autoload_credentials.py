"""Tests for assess_autoload_credentials module."""

import os
import logging
import StringIO
from mock import patch
from tests import TestCase, parse_error

from assess_autoload_credentials import (
    parse_args,
    AWSEnvironment,
    XDGDataPath,
)


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(['/bin/juju'])
        self.assertEqual('/bin/juju', args.juju_bin)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch('sys.stdout', fake_stdout):
                parse_args(['--help'])
        self.assertEqual('', fake_stderr.getvalue())
        self.assertNotIn('TODO', fake_stdout.getvalue())

    def test_verbose_is_set_to_debug_when_passed(self):
        args = parse_args(['/bin/juju', '--verbose'])
        self.assertEqual(logging.DEBUG, args.verbose)

    def test_verbose_defaults_to_INFO(self):
        args = parse_args(['/bin/juju'])
        self.assertEqual(logging.INFO, args.verbose)


class TestXDGDataPath(TestCase):

    def test_XDG_DATA_PATH_is_set(self):
        original_xdg_env = os.environ.get('XDG_DATA_HOME')
        with XDGDataPath() as xdg_path:
            self.assertEqual(os.environ['XDG_DATA_HOME'], xdg_path)
            self.assertNotEqual(os.environ['XDG_DATA_HOME'], original_xdg_env)

    def test_XDG_DATA_PATH_is_reset_afterwards(self):
        original_xdg_env = os.environ.get('XDG_DATA_HOME')
        with XDGDataPath():
            pass
        self.assertEqual(os.environ.get('XDG_DATA_HOME'), original_xdg_env)

    def test_tmp_dir_exists(self):
        with XDGDataPath() as xdg_path:
            self.assertTrue(os.path.exists(xdg_path))

    def test_tmp_dir_is_cleaned_up(self):
        with XDGDataPath() as xdg_path:
            pass
        self.assertFalse(os.path.exists(xdg_path))


class TestAWSEnvironment(TestCase):

    def test_user_access_success_keys_set(self):
        user = 'test_user'
        access_key = 'testing access key'
        secret_key = 'testing secret key'
        with AWSEnvironment(user, access_key, secret_key):
            self.assertEqual(user, os.environ['USER'])
            self.assertEqual(access_key, os.environ['AWS_ACCESS_KEY_ID'])
            self.assertEqual(secret_key, os.environ['AWS_SECRET_ACCESS_KEY'])

    def test_user_access_success_keys_reset(self):
        original_user = os.environ.get('USER')
        original_access_key = os.environ.get('AWS_ACCESS_KEY_ID')
        original_secret_key = os.environ.get('AWS_SECRET_ACCESS_KEY')
        with AWSEnvironment('user', 'access_key', 'secret_key'):
            pass

        self.assertEqual(original_user, os.environ.get('USER'))
        self.assertEqual(
            original_access_key,
            os.environ.get('AWS_ACCESS_KEY_ID')
        )
        self.assertEqual(
            original_secret_key,
            os.environ.get('AWS_SECRET_ACCESS_KEY')
        )
