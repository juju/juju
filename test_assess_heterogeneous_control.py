__metaclass__ = type

from unittest import TestCase

from mock import patch

from assess_heterogeneous_control import dumping_env
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )

class TestDumping_env(TestCase):

    def test_dumping_env_exception(self):
        client = EnvJujuClient(SimpleEnvironment('env'), '5', '4')
        with patch.object(client, 'destroy_environment') as de_mock:
            with patch('subprocess.check_call') as cc_mock:
                with patch('deploy_stack.copy_remote_logs') as crl_mock:
                    with self.assertRaises(ValueError):
                        with dumping_env(client, 'foo', 'bar'):
                            raise ValueError
        crl_mock.assert_called_once_with('foo', 'bar')
        cc_mock.assert_called_once_with(['gzip', '-f'])
        de_mock.assert_called_once_with()

    def test_dumping_env_success(self):
        client = EnvJujuClient(SimpleEnvironment('env'), '5', '4')
        with patch.object(client, 'destroy_environment') as de_mock:
            with patch('subprocess.check_call') as cc_mock:
                with patch('deploy_stack.copy_remote_logs') as crl_mock:
                    with dumping_env(client, 'foo', 'bar'):
                        pass
        crl_mock.assert_called_once_with('foo', 'bar')
        cc_mock.assert_called_once_with(['gzip', '-f'])
        self.assertEqual(de_mock.call_count, 0)
