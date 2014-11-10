import os
from unittest import TestCase

from mock import (
    call,
    patch,
    )

from jujuconfig import (
    get_euca_env,
    translate_to_env,
    )
from jujupy import SimpleEnvironment
from substrate import terminate_instances


def get_aws_env():
    return SimpleEnvironment('baz', {
        'type': 'ec2',
        'region': 'ca-west',
        'access-key': 'skeleton-key',
        'secret-key': 'secret-skeleton-key',
        })


def get_openstack_env():
    return SimpleEnvironment('foo', {
        'type': 'openstack',
        'region': 'ca-west',
        'username': 'steve',
        'password': 'skeleton',
        'tenant-name': 'marcia',
        'auth-url': 'http://example.com',
    })


def get_aws_environ(env):
    environ = dict(os.environ)
    environ.update(get_euca_env(env.config))
    return environ


class TestTerminateInstances(TestCase):

    def test_terminate_aws(self):
        env = get_aws_env()
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                terminate_instances(env, ['foo', 'bar'])
        environ = get_aws_environ(env)
        cc_mock.assert_called_with(
            ['euca-terminate-instances', 'foo', 'bar'], env=environ)
        self.assertEqual(out_mock.write.mock_calls, [
            call('Deleting foo, bar.'), call('\n')])

    def test_terminate_aws_none(self):
        env = get_aws_env()
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                terminate_instances(env, [])
        self.assertEqual(cc_mock.call_count, 0)
        self.assertEqual(out_mock.write.mock_calls, [
            call('No instances to delete.'), call('\n')])

    def test_terminate_openstack(self):
        env = get_openstack_env()
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                terminate_instances(env, ['foo', 'bar'])
        environ = dict(os.environ)
        environ.update(translate_to_env(env.config))
        cc_mock.assert_called_with(
            ['nova', 'delete', 'foo', 'bar'], env=environ)
        self.assertEqual(out_mock.write.mock_calls, [
            call('Deleting foo, bar.'), call('\n')])

    def test_terminate_openstack_none(self):
        env = get_openstack_env()
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                terminate_instances(env, [])
        self.assertEqual(cc_mock.call_count, 0)
        self.assertEqual(out_mock.write.mock_calls, [
            call('No instances to delete.'), call('\n')])

    def test_terminate_uknown(self):
        env = SimpleEnvironment('foo', {'type': 'unknown'})
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                with self.assertRaisesRegexp(
                        ValueError,
                        'This test does not support the unknown provider'):
                    terminate_instances(env, [])
        self.assertEqual(cc_mock.call_count, 0)
        self.assertEqual(out_mock.write.call_count, 0)
