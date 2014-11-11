import os
from unittest import TestCase

from mock import (
    ANY,
    call,
    patch,
    )

from jujuconfig import (
    get_euca_env,
    translate_to_env,
    )
from jujupy import SimpleEnvironment
from substrate import (
    get_libvirt_domstate,
    start_libvirt_domain,
    stop_libvirt_domain,
    terminate_instances,
    verify_libvirt_domain,
)


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


class TestLibvirt(TestCase):

    def test_start_libvirt_domain(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('subprocess.check_output',
                   return_value='running') as mock_sp:
            with patch('substrate.sleep'):
                start_libvirt_domain(URI, dom_name)
        mock_sp.assert_any_call(['virsh', '-c', URI, 'start', dom_name],
                                stderr=ANY)

    def test_stop_libvirt_domain(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('subprocess.check_output',
                   return_value='shut off') as mock_sp:
            with patch('substrate.sleep'):
                stop_libvirt_domain(URI, dom_name)
        mock_sp.assert_any_call(['virsh', '-c', URI, 'shutdown', dom_name],
                                stderr=ANY)

    def test_get_libvirt_domstate(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        expected_cmd = ['virsh', '-c', URI, 'domstate', dom_name]
        with patch('subprocess.check_output') as m_sub:
            get_libvirt_domstate(URI, dom_name)
        m_sub.assert_called_with(expected_cmd)

    def test_verify_libvirt_domain_shut_off_true(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='shut off'):
            rval = verify_libvirt_domain(URI, dom_name, 'shut off')
        self.assertTrue(rval)

    def test_verify_libvirt_domain_shut_off_false(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='running'):
            rval = verify_libvirt_domain(URI, dom_name, 'shut off')
        self.assertFalse(rval)

    def test_verify_libvirt_domain_running_true(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='running'):
            rval = verify_libvirt_domain(URI, dom_name, 'running')
        self.assertTrue(rval)

    def test_verify_libvirt_domain_running_false(self):
        URI = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='shut off'):
            rval = verify_libvirt_domain(URI, dom_name, 'running')
        self.assertFalse(rval)
