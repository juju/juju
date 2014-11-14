import os
from subprocess import CalledProcessError
from unittest import TestCase

from boto.ec2.securitygroup import SecurityGroup
from boto.exception import EC2ResponseError
from mock import (
    ANY,
    call,
    MagicMock,
    patch,
    )

from jujuconfig import (
    get_euca_env,
    translate_to_env,
    )
from jujupy import SimpleEnvironment
from substrate import (
    AWSAccount,
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


class TestAWSAccount(TestCase):

    def test_from_config(self):
        aws = AWSAccount.from_config({
            'access-key': 'skeleton',
            'region': 'france',
            'secret-key': 'hoover',
            })
        self.assertEqual(aws.euca_environ, {
            'EC2_ACCESS_KEY': 'skeleton',
            'EC2_SECRET_KEY': 'hoover',
            'EC2_URL': 'https://france.ec2.amazonaws.com',
            })

    def test_get_environ(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        environ = dict(os.environ)
        environ.update({
            'EC2_ACCESS_KEY': 'skeleton-key',
            'EC2_SECRET_KEY': 'secret-skeleton-key',
            'EC2_URL': 'https://ca-west.ec2.amazonaws.com',
            })
        self.assertEqual(aws.get_environ(), environ)

    def test_euca(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        with patch('subprocess.check_call', return_value='quxx') as co_mock:
            result = aws.euca('foo-bar', ['baz', 'qux'])
        co_mock.assert_called_once_with(['euca-foo-bar', 'baz', 'qux'],
                                        env=aws.get_environ())
        self.assertEqual(result, 'quxx')

    def test_get_euca_output(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        with patch('subprocess.check_output', return_value='quxx') as co_mock:
            result = aws.get_euca_output('foo-bar', ['baz', 'qux'])
        co_mock.assert_called_once_with(['euca-foo-bar', 'baz', 'qux'],
                                        env=aws.get_environ())
        self.assertEqual(result, 'quxx')

    def test_list_security_groups(self):
        aws = AWSAccount.from_config(get_aws_env().config)

        def make_group(name):
            return '\t'.join([
                'GROUP', name + '-id', '689913858002',
                name, 'juju group', 'vpc-1f40b47a'])

        return_value = ''.join(
            make_group(g) + '\n' for g in ['foo', 'foobar', 'baz'])
        return_value += 'RANDOM\n'
        return_value += '\n'
        with patch('subprocess.check_output',
                   return_value=return_value) as co_mock:
            groups = list(aws.list_security_groups())
        co_mock.assert_called_once_with(
            ['euca-describe-groups', '--filter', 'description=juju group'],
            env=aws.get_environ())
        self.assertEqual(groups, [
            ('foo-id', 'foo'), ('foobar-id', 'foobar'), ('baz-id', 'baz')])

    def test_list_instance_security_groups(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        with patch.object(aws, 'get_ec2_connection') as gec_mock:
            instances = [
                MagicMock(instances=[MagicMock(groups=[
                    SecurityGroup(id='foo', name='bar'),
                    ])]),
                MagicMock(instances=[MagicMock(groups=[
                    SecurityGroup(id='baz', name='qux'),
                    SecurityGroup(id='quxx-id', name='quxx'),
                    ])]),
            ]
            gec_mock.return_value.get_all_instances.return_value = instances
            groups = list(aws.list_instance_security_groups())
        gec_mock.assert_called_once_with()
        self.assertEqual(groups, [
            ('foo', 'bar'), ('baz', 'qux'),  ('quxx-id', 'quxx')])

    def test_destroy_security_groups(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        with patch('subprocess.check_call') as cc_mock:
            failures = aws.destroy_security_groups(['foo', 'foobar', 'baz'])
        self.assertEqual(cc_mock.mock_calls[0], call(
            ['euca-delete-group', 'foo'], env=aws.get_environ()))
        self.assertEqual(cc_mock.mock_calls[1], call(
            ['euca-delete-group', 'foobar'], env=aws.get_environ()))
        self.assertEqual(cc_mock.mock_calls[2], call(
            ['euca-delete-group', 'baz'], env=aws.get_environ()))
        self.assertEqual(3, cc_mock.call_count)
        self.assertEqual(failures, [])

    def test_destroy_security_failures(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        with patch('subprocess.check_call',
                   side_effect=CalledProcessError(1, 'foo')):
            failures = aws.destroy_security_groups(['foo', 'foobar', 'baz'])
        self.assertEqual(failures, ['foo', 'foobar', 'baz'])

    def test_get_ec2_connection(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        return_value = object()
        with patch('boto.ec2.connect_to_region',
                   return_value=return_value) as ctr_mock:
            connection = aws.get_ec2_connection()
        ctr_mock.assert_called_once_with(
            'ca-west', aws_access_key_id='skeleton-key',
            aws_secret_access_key='secret-skeleton-key')
        self.assertIs(connection, return_value)

    def make_aws_connection(self):
        aws = AWSAccount.from_config(get_aws_env().config)
        aws.get_ec2_connection = MagicMock()
        connection = aws.get_ec2_connection.return_value
        return aws, connection

    def make_interface(self, group_ids):
        interface = MagicMock()
        interface.groups = [SecurityGroup(id=g) for g in group_ids]
        return interface

    def test_delete_detached_interfaces_with_id(self):
        aws, connection = self.make_aws_connection()
        foo_interface = self.make_interface(['bar-id'])
        baz_interface = self.make_interface(['baz-id', 'bar-id'])
        gani_mock = connection.get_all_network_interfaces
        gani_mock.return_value = [foo_interface, baz_interface]
        unclean = aws.delete_detached_interfaces(['bar-id'])
        gani_mock.assert_called_once_with(
            filters={'status': 'available'})
        foo_interface.delete.assert_called_once_with()
        baz_interface.delete.assert_called_once_with()
        self.assertEqual(unclean, set())

    def test_delete_detached_interfaces_without_id(self):
        aws, connection = self.make_aws_connection()
        baz_interface = self.make_interface(['baz-id'])
        connection.get_all_network_interfaces.return_value = [baz_interface]
        unclean = aws.delete_detached_interfaces(['bar-id'])
        self.assertEqual(baz_interface.delete.call_count, 0)
        self.assertEqual(unclean, set())

    def prepare_delete_exception(self, error_code):
        aws, connection = self.make_aws_connection()
        baz_interface = self.make_interface(['bar-id'])
        e = EC2ResponseError('status', 'reason')
        e.error_code = error_code
        baz_interface.delete.side_effect = e
        connection.get_all_network_interfaces.return_value = [baz_interface]
        return aws, baz_interface

    def test_delete_detached_interfaces_in_use(self):
        aws, baz_interface = self.prepare_delete_exception(
            'InvalidNetworkInterface.InUse')
        unclean = aws.delete_detached_interfaces(['bar-id', 'foo-id'])
        baz_interface.delete.assert_called_once_with()
        self.assertEqual(unclean, set(['bar-id']))

    def test_delete_detached_interfaces_not_found(self):
        aws, baz_interface = self.prepare_delete_exception(
            'InvalidNetworkInterfaceID.NotFound')
        unclean = aws.delete_detached_interfaces(['bar-id', 'foo-id'])
        baz_interface.delete.assert_called_once_with()
        self.assertEqual(unclean, set(['bar-id']))

    def test_delete_detached_interfaces_other(self):
        aws, baz_interface = self.prepare_delete_exception(
            'InvalidNetworkInterfaceID')
        with self.assertRaises(EC2ResponseError):
            unclean = aws.delete_detached_interfaces(['bar-id', 'foo-id'])


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
