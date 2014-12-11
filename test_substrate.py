import os
from subprocess import CalledProcessError
from unittest import TestCase

from boto.ec2.securitygroup import SecurityGroup
from boto.exception import EC2ResponseError
from mock import (
    ANY,
    call,
    MagicMock,
    Mock,
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
    JoyentAccount,
    OpenStackAccount,
    make_substrate,
    start_libvirt_domain,
    StillProvisioning,
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


def get_maas_env():
    return SimpleEnvironment('mas', {
        'type': 'maas',
        'maas-server': 'http://10.0.10.10/MAAS/',
        'maas-oauth': 'a:password:string',
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

    def test_terminate_maas(self):
        env = get_maas_env()
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                terminate_instances(env, ['/A/B/C/D/node-3d/'])
        expected = (
            ['maas', 'login', 'mas', 'http://10.0.10.10/MAAS/api/1.0/',
             'a:password:string'],
        )
        self.assertEqual(expected, cc_mock.call_args_list[0][0])
        expected = (['maas', 'mas', 'node', 'release', 'node-3d'],)
        self.assertEqual(expected, cc_mock.call_args_list[1][0])
        expected = (['maas', 'logout', 'mas'],)
        self.assertEqual(expected, cc_mock.call_args_list[2][0])
        self.assertEqual(3, len(cc_mock.call_args_list))
        self.assertEqual(out_mock.write.mock_calls, [
            call('Deleting /A/B/C/D/node-3d/.'), call('\n')])

    def test_terminate_maas_none(self):
        env = get_maas_env()
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

    def test_terminate_joyent(self):
        with patch('substrate.JoyentAccount.terminate_instances') as ti_mock:
            terminate_instances(
                SimpleEnvironment('foo', get_joyent_config()), ['ab', 'cd'])
        ti_mock.assert_called_once_with(['ab', 'cd'])

    def test_terminate_uknown(self):
        env = SimpleEnvironment('foo', {'type': 'unknown'})
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                with self.assertRaisesRegexp(
                        ValueError,
                        'This test does not support the unknown provider'):
                    terminate_instances(env, ['foo'])
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
        self.assertEqual(aws.region, 'france')

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

    def test_iter_security_groups(self):
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
            groups = list(aws.iter_security_groups())
        co_mock.assert_called_once_with(
            ['euca-describe-groups', '--filter', 'description=juju group'],
            env=aws.get_environ())
        self.assertEqual(groups, [
            ('foo-id', 'foo'), ('foobar-id', 'foobar'), ('baz-id', 'baz')])

    def test_iter_instance_security_groups(self):
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
            gai_mock = gec_mock.return_value.get_all_instances
            gai_mock.return_value = instances
            groups = list(aws.iter_instance_security_groups())
        gec_mock.assert_called_once_with()
        self.assertEqual(groups, [
            ('foo', 'bar'), ('baz', 'qux'),  ('quxx-id', 'quxx')])
        gai_mock.assert_called_once_with(instance_ids=None)

    def test_iter_instance_security_groups_instances(self):
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
            gai_mock = gec_mock.return_value.get_all_instances
            gai_mock.return_value = instances
            list(aws.iter_instance_security_groups(['abc', 'def']))
        gai_mock.assert_called_once_with(instance_ids=['abc', 'def'])

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
            aws.delete_detached_interfaces(['bar-id', 'foo-id'])


def get_os_config():
    return {
        'type': 'openstack', 'username': 'foo', 'password': 'bar',
        'tenant-name': 'baz', 'auth-url': 'qux', 'region': 'quxx'}


def make_os_security_groups(names, non_juju=()):
    groups = []
    for name in names:
        group = Mock(id='{}-id'.format(name))
        group.name = name
        if name in non_juju:
            group.description = 'asdf'
        else:
            group.description = 'juju group'
        groups.append(group)
    return groups


def make_os_security_group_instance(names):
    instance_id = '-'.join(names) + '-id'
    return MagicMock(
        id=instance_id, security_groups=[{'name': n} for n in names])


class TestOpenstackAccount(TestCase):

    def test_from_config(self):
        account = OpenStackAccount.from_config(get_os_config())
        self.assertEqual(account._username, 'foo')
        self.assertEqual(account._password, 'bar')
        self.assertEqual(account._tenant_name, 'baz')
        self.assertEqual(account._auth_url, 'qux')
        self.assertEqual(account._region_name, 'quxx')

    def test_get_client(self):
        account = OpenStackAccount.from_config(get_os_config())
        with patch('novaclient.client.Client') as ncc_mock:
            account.get_client()
        ncc_mock.assert_called_once_with(
            '1.1', 'foo', 'bar', 'baz', 'qux', region_name='quxx',
            service_type='compute', insecure=False)

    def test_iter_security_groups(self):
        account = OpenStackAccount.from_config(get_os_config())
        with patch.object(account, 'get_client') as gc_mock:
            client = gc_mock.return_value
            groups = make_os_security_groups(['foo', 'bar', 'baz'])
            client.security_groups.list.return_value = groups
            result = account.iter_security_groups()
        self.assertEqual(list(result), [
            ('foo-id', 'foo'), ('bar-id', 'bar'), ('baz-id', 'baz')])

    def test_iter_security_groups_non_juju(self):
        account = OpenStackAccount.from_config(get_os_config())
        with patch.object(account, 'get_client') as gc_mock:
            client = gc_mock.return_value
            groups = make_os_security_groups(
                ['foo', 'bar', 'baz'], non_juju=['foo', 'baz'])
            client.security_groups.list.return_value = groups
            result = account.iter_security_groups()
        self.assertEqual(list(result), [('bar-id', 'bar')])

    def test_iter_instance_security_groups(self):
        account = OpenStackAccount.from_config(get_os_config())
        with patch.object(account, 'get_client') as gc_mock:
            client = gc_mock.return_value
            instance = MagicMock(security_groups=[{'name': 'foo'}])
            client.servers.list.return_value = [instance]
            groups = make_os_security_groups(['foo', 'bar'])
            client.security_groups.list.return_value = groups
            result = account.iter_instance_security_groups()
        self.assertEqual(list(result), [('foo-id', 'foo')])

    def test_iter_instance_security_groups_instance_ids(self):
        account = OpenStackAccount.from_config(get_os_config())
        with patch.object(account, 'get_client') as gc_mock:
            client = gc_mock.return_value
            foo_bar = make_os_security_group_instance(['foo', 'bar'])
            baz_bar = make_os_security_group_instance(['baz', 'bar'])
            client.servers.list.return_value = [foo_bar, baz_bar]
            groups = make_os_security_groups(['foo', 'bar', 'baz'])
            client.security_groups.list.return_value = groups
            result = account.iter_instance_security_groups(['foo-bar-id'])
        self.assertEqual(list(result), [('foo-id', 'foo'), ('bar-id', 'bar')])


def get_joyent_config():
    return {
        'type': 'joyent',
        'sdc-url': 'http://example.org/sdc',
        'manta-user': 'user@manta.org',
        'manta-key-id': 'key-id@manta.org',
        }


class TestJoyentAccount(TestCase):

    def test_from_config(self):
        account = JoyentAccount.from_config(get_joyent_config())
        self.assertEqual(account.client.sdc_url, 'http://example.org/sdc')
        self.assertEqual(account.client.account, 'user@manta.org')
        self.assertEqual(account.client.key_id, 'key-id@manta.org')

    def test_terminate_instances(self):
        client = Mock()
        account = JoyentAccount(client)
        client._list_machines.return_value = {'state': 'stopped'}
        account.terminate_instances(['asdf'])
        client.stop_machine.assert_called_once_with('asdf')
        self.assertEqual(client._list_machines.mock_calls,
                         [call('asdf'), call('asdf')])
        client.delete_machine.assert_called_once_with('asdf')

    def test_terminate_instances_waits_for_stopped(self):
        client = Mock()
        account = JoyentAccount(client)
        machines = iter([{'state': 'foo'}, {'state': 'bar'},
                         {'state': 'stopped'}])
        client._list_machines.side_effect = lambda x: machines.next()
        with patch('substrate.sleep'):
            account.terminate_instances(['asdf'])
        client.stop_machine.assert_called_once_with('asdf')
        self.assertEqual(client._list_machines.call_count, 3)
        client.delete_machine.assert_called_once_with('asdf')

    def test_terminate_instances_stop_failure(self):
        client = Mock()
        account = JoyentAccount(client)
        client._list_machines.return_value = {'state': 'foo'}
        with patch('substrate.sleep'):
            with patch('substrate.until_timeout', return_value=[]):
                with self.assertRaisesRegexp(
                        Exception, 'Instance did not stop: asdf'):
                    account.terminate_instances(['asdf'])

    def test_terminate_instances_still_provisioning(self):
        client = Mock()
        account = JoyentAccount(client)
        machines = {
            'a': {'state': 'stopped'},
            'b': {'state': 'provisioning'},
            'c': {'state': 'provisioning'},
            }
        client._list_machines.side_effect = machines.get
        with self.assertRaises(StillProvisioning) as exc:
            account.terminate_instances(['b', 'c', 'a'])
        self.assertEqual(exc.exception.instance_ids, ['b', 'c'])
        client.delete_machine.assert_called_once_with('a')


class TestMakeSubstrate(TestCase):

    def test_make_substrate_aws(self):
        aws_env = get_aws_env()
        aws = make_substrate(aws_env.config)
        self.assertIs(type(aws), AWSAccount)
        self.assertEqual(aws.euca_environ, {
            'EC2_ACCESS_KEY': 'skeleton-key',
            'EC2_SECRET_KEY': 'secret-skeleton-key',
            'EC2_URL': 'https://ca-west.ec2.amazonaws.com',
            })
        self.assertEqual(aws.region, 'ca-west')

    def test_make_substrate_openstack(self):
        config = get_os_config()
        account = make_substrate(config)
        self.assertIs(type(account), OpenStackAccount)
        self.assertEqual(account._username, 'foo')
        self.assertEqual(account._password, 'bar')
        self.assertEqual(account._tenant_name, 'baz')
        self.assertEqual(account._auth_url, 'qux')
        self.assertEqual(account._region_name, 'quxx')

    def test_make_substrate_joyent(self):
        config = get_joyent_config()
        account = make_substrate(config)
        self.assertEqual(account.client.sdc_url, 'http://example.org/sdc')
        self.assertEqual(account.client.account, 'user@manta.org')
        self.assertEqual(account.client.key_id, 'key-id@manta.org')

    def test_make_substrate_other(self):
        config = get_os_config()
        config['type'] = 'other'
        self.assertIs(make_substrate(config), None)


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
