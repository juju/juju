from contextlib import contextmanager
import json
import os
from subprocess import CalledProcessError
from textwrap import dedent
from unittest import TestCase

from boto.ec2.securitygroup import SecurityGroup
from boto.exception import EC2ResponseError
from mock import (
    ANY,
    call,
    create_autospec,
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
    AzureAccount,
    describe_instances,
    destroy_job_instances,
    get_job_instances,
    get_libvirt_domstate,
    JoyentAccount,
    LXDAccount,
    make_substrate_manager,
    MAASAccount,
    OpenStackAccount,
    parse_euca,
    run_instances,
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


def get_lxd_env():
    return SimpleEnvironment('mas', {
        'type': 'lxd'
        })


def get_maas_env():
    return SimpleEnvironment('mas', {
        'type': 'maas',
        'maas-server': 'http://10.0.10.10/MAAS/',
        'maas-oauth': 'a:password:string',
        'name': 'mas'
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


def get_rax_env():
    return SimpleEnvironment('rax', {
        'type': 'rackspace',
        'region': 'DFW',
        'username': 'a-user',
        'password': 'a-pasword',
        'tenant-name': '100',
        'auth-url': 'http://rax.testing',
    })


def get_aws_environ(env):
    environ = dict(os.environ)
    environ.update(get_euca_env(env.config))
    return environ


def make_maas_node(hostname='juju-qa-maas-node-1.maas'):
    return {
        "status": 6,
        "macaddress_set": [
            {
                "resource_uri": "/MAAS/api/1.0/nodes/node-0123a-4567-890a",
                "mac_address": "52:54:00:71:84:bc"
            }
        ],
        "hostname": hostname,
        "zone": {
            "resource_uri": "/MAAS/api/1.0/zones/default/",
            "name": "default",
            "description": ""
        },
        "routers": [
            "e4:11:5b:0e:74:ac",
            "fe:54:00:71:84:bc"
        ],
        "netboot": True,
        "cpu_count": 1,
        "storage": 1408,
        "owner": "root",
        "system_id": "node-75e0d560-7432-11e4-bb28-525400c43ce5",
        "architecture": "amd64/generic",
        "memory": 2048,
        "power_type": "virsh",
        "tag_names": [
            "virtual"
        ],
        "ip_addresses": [
            "10.0.30.165"
        ],
        "resource_uri": "/MAAS/api/1.0/nodes/node-0123a-4567-890a"
    }


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

    def test_terminate_rackspace(self):
        env = get_rax_env()
        with patch('subprocess.check_call') as cc_mock:
            with patch('sys.stdout') as out_mock:
                terminate_instances(env, ['foo', 'bar'])
        environ = dict(os.environ)
        environ.update(translate_to_env(env.config))
        cc_mock.assert_called_with(
            ['nova', 'delete', 'foo', 'bar'], env=environ)
        self.assertEqual(out_mock.write.mock_calls, [
            call('Deleting foo, bar.'), call('\n')])

    def test_terminate_joyent(self):
        with patch('substrate.JoyentAccount.terminate_instances') as ti_mock:
            terminate_instances(
                SimpleEnvironment('foo', get_joyent_config()), ['ab', 'cd'])
        ti_mock.assert_called_once_with(['ab', 'cd'])

    def test_terminate_lxd(self):
        env = get_lxd_env()
        with patch('subprocess.check_call') as cc_mock:
            terminate_instances(env, ['foo', 'bar'])
        self.assertEqual(
            [call(['lxc', 'stop', '--force', 'foo']),
             call(['lxc', 'delete', '--force', 'foo']),
             call(['lxc', 'stop', '--force', 'bar']),
             call(['lxc', 'delete', '--force', 'bar'])],
            cc_mock.mock_calls)

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

    def test_manager_from_config(self):
        with AWSAccount.manager_from_config({
                'access-key': 'skeleton',
                'region': 'france',
                'secret-key': 'hoover',
                }) as aws:
            self.assertEqual(aws.euca_environ, {
                'AWS_ACCESS_KEY': 'skeleton',
                'AWS_SECRET_KEY': 'hoover',
                'EC2_ACCESS_KEY': 'skeleton',
                'EC2_SECRET_KEY': 'hoover',
                'EC2_URL': 'https://france.ec2.amazonaws.com',
                })
            self.assertEqual(aws.region, 'france')

    def test_iter_security_groups(self):

        def make_group():
            class FakeGroup:
                def __init__(self, name):
                    self.name, self.id = name, name + "-id"

            for name in ['foo', 'foobar', 'baz']:
                group = FakeGroup(name)
                yield group

        client = MagicMock(spec=['get_all_security_groups'])
        client.get_all_security_groups.return_value = list(make_group())
        with patch('substrate.ec2.connect_to_region',
                   return_value=client) as ctr_mock:
            with AWSAccount.manager_from_config(get_aws_env().config) as aws:
                groups = list(aws.iter_security_groups())
        self.assertEqual(groups, [
            ('foo-id', 'foo'), ('foobar-id', 'foobar'), ('baz-id', 'baz')])
        self.assert_ec2_connection_call(ctr_mock)

    def assert_ec2_connection_call(self, ctr_mock):
        ctr_mock.assert_called_once_with(
            'ca-west', aws_access_key_id='skeleton-key',
            aws_secret_access_key='secret-skeleton-key')

    def test_iter_instance_security_groups(self):
        instances = [
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='foo', name='bar'), ])]),
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='baz', name='qux'),
                SecurityGroup(id='quxx-id', name='quxx'), ])]),
        ]
        client = MagicMock(spec=['get_all_instances'])
        client.get_all_instances.return_value = instances
        with patch('substrate.ec2.connect_to_region',
                   return_value=client) as ctr_mock:
            with AWSAccount.manager_from_config(get_aws_env().config) as aws:
                groups = list(aws.iter_instance_security_groups())
        self.assertEqual(
            groups, [('foo', 'bar'), ('baz', 'qux'), ('quxx-id', 'quxx')])
        client.get_all_instances.assert_called_once_with(instance_ids=None)
        self.assert_ec2_connection_call(ctr_mock)

    def test_iter_instance_security_groups_instances(self):
        instances = [
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='foo', name='bar'),
                ])]),
            MagicMock(instances=[MagicMock(groups=[
                SecurityGroup(id='baz', name='qux'),
                SecurityGroup(id='quxx-id', name='quxx'),
                ])]),
        ]
        client = MagicMock(spec=['get_all_instances'])
        client.get_all_instances.return_value = instances
        with patch('substrate.ec2.connect_to_region',
                   return_value=client) as ctr_mock:
            with AWSAccount.manager_from_config(get_aws_env().config) as aws:
                    list(aws.iter_instance_security_groups(['abc', 'def']))
        client.get_all_instances.assert_called_once_with(
            instance_ids=['abc', 'def'])
        self.assert_ec2_connection_call(ctr_mock)

    def test_destroy_security_groups(self):
        client = MagicMock(spec=['delete_security_group'])
        client.delete_security_group.return_value = True
        with patch('substrate.ec2.connect_to_region',
                   return_value=client) as ctr_mock:
            with AWSAccount.manager_from_config(get_aws_env().config) as aws:
                failures = aws.destroy_security_groups(
                    ['foo', 'foobar', 'baz'])
        calls = [call(name='foo'), call(name='foobar'), call(name='baz')]
        self.assertEqual(client.delete_security_group.mock_calls, calls)
        self.assertEqual(failures, [])
        self.assert_ec2_connection_call(ctr_mock)

    def test_destroy_security_failures(self):
        client = MagicMock(spec=['delete_security_group'])
        client.delete_security_group.return_value = False
        with patch('substrate.ec2.connect_to_region',
                   return_value=client) as ctr_mock:
            with AWSAccount.manager_from_config(get_aws_env().config) as aws:
                failures = aws.destroy_security_groups(
                    ['foo', 'foobar', 'baz'])
        self.assertEqual(failures, ['foo', 'foobar', 'baz'])
        self.assert_ec2_connection_call(ctr_mock)

    @contextmanager
    def make_aws_connection(self, return_value):
        client = MagicMock(spec=['get_all_network_interfaces'])
        client.get_all_network_interfaces.return_value = return_value
        with patch('substrate.ec2.connect_to_region',
                   return_value=client) as ctr_mock:
            with AWSAccount.manager_from_config(get_aws_env().config) as aws:
                yield aws
        self.assert_ec2_connection_call(ctr_mock)

    def make_interface(self, group_ids):
        interface = MagicMock(spec=['groups', 'delete', 'id'])
        interface.groups = [SecurityGroup(id=g) for g in group_ids]
        return interface

    def test_delete_detached_interfaces_with_id(self):
        foo_interface = self.make_interface(['bar-id'])
        baz_interface = self.make_interface(['baz-id', 'bar-id'])
        with self.make_aws_connection([foo_interface, baz_interface]) as aws:
            unclean = aws.delete_detached_interfaces(['bar-id'])
            foo_interface.delete.assert_called_once_with()
            baz_interface.delete.assert_called_once_with()
        self.assertEqual(unclean, set())

    def test_delete_detached_interfaces_without_id(self):
        baz_interface = self.make_interface(['baz-id'])
        with self.make_aws_connection([baz_interface]) as aws:
            unclean = aws.delete_detached_interfaces(['bar-id'])
        self.assertEqual(baz_interface.delete.call_count, 0)
        self.assertEqual(unclean, set())

    def prepare_delete_exception(self, error_code):
        baz_interface = self.make_interface(['bar-id'])
        e = EC2ResponseError('status', 'reason')
        e.error_code = error_code
        baz_interface.delete.side_effect = e
        return baz_interface

    def test_delete_detached_interfaces_in_use(self):
        baz_interface = self.prepare_delete_exception(
            'InvalidNetworkInterface.InUse')
        with self.make_aws_connection([baz_interface]) as aws:
            unclean = aws.delete_detached_interfaces(['bar-id', 'foo-id'])
        baz_interface.delete.assert_called_once_with()
        self.assertEqual(unclean, set(['bar-id']))

    def test_delete_detached_interfaces_not_found(self):
        baz_interface = self.prepare_delete_exception(
            'InvalidNetworkInterfaceID.NotFound')
        with self.make_aws_connection([baz_interface]) as aws:
            unclean = aws.delete_detached_interfaces(['bar-id', 'foo-id'])
        baz_interface.delete.assert_called_once_with()
        self.assertEqual(unclean, set(['bar-id']))

    def test_delete_detached_interfaces_other(self):
        baz_interface = self.prepare_delete_exception(
            'InvalidNetworkInterfaceID')
        with self.make_aws_connection([baz_interface]) as aws:
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

    def test_manager_from_config(self):
        with OpenStackAccount.manager_from_config(get_os_config()) as account:
            self.assertEqual(account._username, 'foo')
            self.assertEqual(account._password, 'bar')
            self.assertEqual(account._tenant_name, 'baz')
            self.assertEqual(account._auth_url, 'qux')
            self.assertEqual(account._region_name, 'quxx')

    def test_get_client(self):
        with OpenStackAccount.manager_from_config(get_os_config()) as account:
            with patch('novaclient.client.Client') as ncc_mock:
                account.get_client()
        ncc_mock.assert_called_once_with(
            '1.1', 'foo', 'bar', 'baz', 'qux', region_name='quxx',
            service_type='compute', insecure=False)

    def test_iter_security_groups(self):
        with OpenStackAccount.manager_from_config(get_os_config()) as account:
            with patch.object(account, 'get_client') as gc_mock:
                client = gc_mock.return_value
                groups = make_os_security_groups(['foo', 'bar', 'baz'])
                client.security_groups.list.return_value = groups
                result = account.iter_security_groups()
            self.assertEqual(list(result), [
                ('foo-id', 'foo'), ('bar-id', 'bar'), ('baz-id', 'baz')])

    def test_iter_security_groups_non_juju(self):
        with OpenStackAccount.manager_from_config(get_os_config()) as account:
            with patch.object(account, 'get_client') as gc_mock:
                client = gc_mock.return_value
                groups = make_os_security_groups(
                    ['foo', 'bar', 'baz'], non_juju=['foo', 'baz'])
                client.security_groups.list.return_value = groups
                result = account.iter_security_groups()
            self.assertEqual(list(result), [('bar-id', 'bar')])

    def test_iter_instance_security_groups(self):
        with OpenStackAccount.manager_from_config(get_os_config()) as account:
            with patch.object(account, 'get_client') as gc_mock:
                client = gc_mock.return_value
                instance = MagicMock(security_groups=[{'name': 'foo'}])
                client.servers.list.return_value = [instance]
                groups = make_os_security_groups(['foo', 'bar'])
                client.security_groups.list.return_value = groups
                result = account.iter_instance_security_groups()
            self.assertEqual(list(result), [('foo-id', 'foo')])

    def test_iter_instance_security_groups_instance_ids(self):
        with OpenStackAccount.manager_from_config(get_os_config()) as account:
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
        'manta-url': 'http://us-east.manta.example.org',
        'private-key': 'key\abc\n'
        }


class TestJoyentAccount(TestCase):

    def test_manager_from_config(self):
        with JoyentAccount.manager_from_config(get_joyent_config()) as account:
            self.assertEqual(
                open(account.client.key_path).read(), 'key\abc\n')
        self.assertFalse(os.path.exists(account.client.key_path))
        self.assertTrue(account.client.key_path.endswith('joyent.key'))
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


def get_lxd_config():
    return {'type': 'lxd'}


class TestLXDAccount(TestCase):

    def test_manager_from_config(self):
        config = get_lxd_config()
        with LXDAccount.manager_from_config(config) as account:
            self.assertIsNone(account.remote)
        config['region'] = 'lxd-server'
        with LXDAccount.manager_from_config(config) as account:
            self.assertEqual('lxd-server', account.remote)

    def test_terminate_instances(self):
        account = LXDAccount()
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            account.terminate_instances(['asdf'])
        self.assertEqual(
            [call(['lxc', 'stop', '--force', 'asdf']),
             call(['lxc', 'delete', '--force', 'asdf'])],
            cc_mock.mock_calls)

    def test_terminate_instances_with_remote(self):
        account = LXDAccount(remote='lxd-server')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            account.terminate_instances(['asdf'])
        self.assertEqual(
            [call(['lxc', 'stop', '--force', 'asdf']),
             call(['lxc', 'delete', '--force', 'lxd-server:asdf'])],
            cc_mock.mock_calls)


def make_sms(instance_ids):
    from azure import servicemanagement as sm
    client = create_autospec(sm.ServiceManagementService('foo', 'bar'))

    services = AzureAccount.convert_instance_ids(instance_ids)

    def get_hosted_service_properties(service, embed_detail):
        props = sm.HostedService()
        deployment = sm.Deployment()
        deployment.name = service + '-v3'
        for role_name in services[service]:
            role = sm.Role()
            role.role_name = role_name
            deployment.role_list.roles.append(role)
        props.deployments.deployments.append(deployment)
        return props

    client.get_hosted_service_properties.side_effect = (
        get_hosted_service_properties)
    client.get_operation_status.return_value = Mock(status='Succeeded')
    client.delete_role.return_value = sm.AsynchronousOperationResult()
    return client


class TestAzureAccount(TestCase):

    def test_manager_from_config(self):
        config = {'management-subscription-id': 'fooasdfbar',
                  'management-certificate': 'ab\ncd\n'}
        with AzureAccount.manager_from_config(config) as substrate:
            self.assertEqual(substrate.service_client.subscription_id,
                             'fooasdfbar')
            self.assertEqual(open(substrate.service_client.cert_file).read(),
                             'ab\ncd\n')
        self.assertFalse(os.path.exists(substrate.service_client.cert_file))

    def test_convert_instance_ids(self):
        converted = AzureAccount.convert_instance_ids([
            'foo-bar-baz', 'foo-bar-qux', 'foo-noo-baz'])
        self.assertEqual(converted, {
            'foo-bar': {'baz', 'qux'},
            'foo-noo': {'baz'},
            })

    def test_terminate_instances_one_role(self):
        client = make_sms(['foo-bar'])
        account = AzureAccount(client)
        account.terminate_instances(['foo-bar'])
        client.delete_deployment.assert_called_once_with('foo', 'foo-v3')
        client.delete_hosted_service.assert_called_once_with('foo')

    def test_terminate_instances_not_all_roles(self):
        client = make_sms(['foo-bar', 'foo-baz', 'foo-qux'])
        account = AzureAccount(client)
        account.terminate_instances(['foo-bar', 'foo-baz'])
        client.get_hosted_service_properties.assert_called_once_with(
            'foo', embed_detail=True)
        self.assertItemsEqual(client.delete_role.mock_calls, [
            call('foo', 'foo-v3', 'bar'),
            call('foo', 'foo-v3', 'baz'),
            ])
        self.assertEqual(client.delete_deployment.call_count, 0)
        self.assertEqual(client.delete_hosted_service.call_count, 0)

    def test_terminate_instances_all_roles(self):
        client = make_sms(['foo-bar', 'foo-baz', 'foo-qux'])
        account = AzureAccount(client)
        account.terminate_instances(['foo-bar', 'foo-baz', 'foo-qux'])
        client.get_hosted_service_properties.assert_called_once_with(
            'foo', embed_detail=True)
        client.delete_deployment.assert_called_once_with('foo', 'foo-v3')
        client.delete_hosted_service.assert_called_once_with('foo')


class TestMAASAcount(TestCase):

    @patch.object(MAASAccount, 'logout', autospec=True)
    @patch.object(MAASAccount, 'login', autospec=True)
    def test_manager_from_config(self, li_mock, lo_mock):
        config = get_maas_env().config
        with MAASAccount.manager_from_config(config) as account:
            self.assertEqual(account.profile, 'mas')
            self.assertEqual(account.url, 'http://10.0.10.10/MAAS/api/1.0/')
            self.assertEqual(account.oauth, 'a:password:string')
            # As the class object is patched, the mocked methods
            # show that self is passed.
            li_mock.assert_called_once_with(account)
        lo_mock.assert_called_once_with(account)

    @patch('subprocess.check_call', autospec=True)
    def test_login(self, cc_mock):
        config = get_maas_env().config
        account = MAASAccount(
            config['name'], config['maas-server'], config['maas-oauth'])
        account.login()
        cc_mock.assert_called_once_with([
            'maas', 'login', 'mas', 'http://10.0.10.10/MAAS/api/1.0/',
            'a:password:string'])

    @patch('subprocess.check_call', autospec=True)
    def test_logout(self, cc_mock):
        config = get_maas_env().config
        account = MAASAccount(
            config['name'], config['maas-server'], config['maas-oauth'])
        account.logout()
        cc_mock.assert_called_once_with(['maas', 'logout', 'mas'])

    @patch('subprocess.check_call', autospec=True)
    def test_terminate_instances(self, cc_mock):
        config = get_maas_env().config
        account = MAASAccount(
            config['name'], config['maas-server'], config['maas-oauth'])
        instance_ids = ['/A/B/C/D/node-1d/', '/A/B/C/D/node-2d/']
        account.terminate_instances(instance_ids)
        cc_mock.assert_any_call(
            ['maas', 'mas', 'node', 'release', 'node-1d'])
        cc_mock.assert_called_with(
            ['maas', 'mas', 'node', 'release', 'node-2d'])

    @patch('subprocess.check_call', autospec=True)
    def test_get_allocated_nodes(self, cc_mock):
        config = get_maas_env().config
        account = MAASAccount(
            config['name'], config['maas-server'], config['maas-oauth'])
        node = make_maas_node('maas-node-1.maas')
        allocated_nodes_string = '[%s]' % json.dumps(node)
        with patch('subprocess.check_output', autospec=True,
                   return_value=allocated_nodes_string) as co_mock:
            allocated = account.get_allocated_nodes()
        co_mock.assert_called_once_with(
            ['maas', 'mas', 'nodes', 'list-allocated'])
        self.assertEqual(node, allocated['maas-node-1.maas'])

    @patch('subprocess.check_call', autospec=True)
    def test_get_allocated_ips(self, cc_mock):
        config = get_maas_env().config
        account = MAASAccount(
            config['name'], config['maas-server'], config['maas-oauth'])
        node = make_maas_node('maas-node-1.maas')
        allocated_nodes_string = '[%s]' % json.dumps(node)
        with patch('subprocess.check_output', autospec=True,
                   return_value=allocated_nodes_string):
            ips = account.get_allocated_ips()
        self.assertEqual('10.0.30.165', ips['maas-node-1.maas'])

    @patch('subprocess.check_call', autospec=True)
    def test_get_allocated_ips_empty(self, cc_mock):
        config = get_maas_env().config
        account = MAASAccount(
            config['name'], config['maas-server'], config['maas-oauth'])
        node = make_maas_node('maas-node-1.maas')
        node['ip_addresses'] = []
        allocated_nodes_string = '[%s]' % json.dumps(node)
        with patch('subprocess.check_output', autospec=True,
                   return_value=allocated_nodes_string):
            ips = account.get_allocated_ips()
        self.assertEqual({}, ips)


class TestMakeSubstrateManager(TestCase):

    def test_make_substrate_manager_aws(self):
        aws_env = get_aws_env()
        with make_substrate_manager(aws_env.config) as aws:
            self.assertIs(type(aws), AWSAccount)
            self.assertEqual(aws.euca_environ, {
                'AWS_ACCESS_KEY': 'skeleton-key',
                'AWS_SECRET_KEY': 'secret-skeleton-key',
                'EC2_ACCESS_KEY': 'skeleton-key',
                'EC2_SECRET_KEY': 'secret-skeleton-key',
                'EC2_URL': 'https://ca-west.ec2.amazonaws.com',
                })
            self.assertEqual(aws.region, 'ca-west')

    def test_make_substrate_manager_openstack(self):
        config = get_os_config()
        with make_substrate_manager(config) as account:
            self.assertIs(type(account), OpenStackAccount)
            self.assertEqual(account._username, 'foo')
            self.assertEqual(account._password, 'bar')
            self.assertEqual(account._tenant_name, 'baz')
            self.assertEqual(account._auth_url, 'qux')
            self.assertEqual(account._region_name, 'quxx')

    def test_make_substrate_manager_rackspace(self):
        config = get_os_config()
        config['type'] = 'rackspace'
        with make_substrate_manager(config) as account:
            self.assertIs(type(account), OpenStackAccount)
            self.assertEqual(account._username, 'foo')
            self.assertEqual(account._password, 'bar')
            self.assertEqual(account._tenant_name, 'baz')
            self.assertEqual(account._auth_url, 'qux')
            self.assertEqual(account._region_name, 'quxx')

    def test_make_substrate_manager_joyent(self):
        config = get_joyent_config()
        with make_substrate_manager(config) as account:
            self.assertEqual(account.client.sdc_url, 'http://example.org/sdc')
            self.assertEqual(account.client.account, 'user@manta.org')
            self.assertEqual(account.client.key_id, 'key-id@manta.org')

    def test_make_substrate_manager_azure(self):
        config = {
            'type': 'azure',
            'management-subscription-id': 'fooasdfbar',
            'management-certificate': 'ab\ncd\n'
            }
        with make_substrate_manager(config) as substrate:
            self.assertIs(type(substrate), AzureAccount)
            self.assertEqual(substrate.service_client.subscription_id,
                             'fooasdfbar')
            self.assertEqual(open(substrate.service_client.cert_file).read(),
                             'ab\ncd\n')
        self.assertFalse(os.path.exists(substrate.service_client.cert_file))

    def test_make_substrate_manager_other(self):
        config = get_os_config()
        config['type'] = 'other'
        with make_substrate_manager(config) as account:
            self.assertIs(account, None)


class TestLibvirt(TestCase):

    def test_start_libvirt_domain(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('subprocess.check_output',
                   return_value='running') as mock_sp:
            with patch('substrate.sleep'):
                start_libvirt_domain(uri, dom_name)
        mock_sp.assert_any_call(['virsh', '-c', uri, 'start', dom_name],
                                stderr=ANY)

    def test_stop_libvirt_domain(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('subprocess.check_output',
                   return_value='shut off') as mock_sp:
            with patch('substrate.sleep'):
                stop_libvirt_domain(uri, dom_name)
        mock_sp.assert_any_call(['virsh', '-c', uri, 'shutdown', dom_name],
                                stderr=ANY)

    def test_get_libvirt_domstate(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        expected_cmd = ['virsh', '-c', uri, 'domstate', dom_name]
        with patch('subprocess.check_output') as m_sub:
            get_libvirt_domstate(uri, dom_name)
        m_sub.assert_called_with(expected_cmd)

    def test_verify_libvirt_domain_shut_off_true(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='shut off'):
            rval = verify_libvirt_domain(uri, dom_name, 'shut off')
        self.assertTrue(rval)

    def test_verify_libvirt_domain_shut_off_false(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='running'):
            rval = verify_libvirt_domain(uri, dom_name, 'shut off')
        self.assertFalse(rval)

    def test_verify_libvirt_domain_running_true(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='running'):
            rval = verify_libvirt_domain(uri, dom_name, 'running')
        self.assertTrue(rval)

    def test_verify_libvirt_domain_running_false(self):
        uri = 'qemu+ssh://someHost/system'
        dom_name = 'fido'
        with patch('substrate.get_libvirt_domstate', return_value='shut off'):
            rval = verify_libvirt_domain(uri, dom_name, 'running')
        self.assertFalse(rval)


class EucaTestCase(TestCase):

    def test_get_job_instances_none(self):
        with patch('substrate.describe_instances',
                   return_value=[], autospec=True) as di_mock:
            ids = get_job_instances('foo')
        self.assertEqual([], [i for i in ids])
        di_mock.assert_called_with(job_name='foo', running=True)

    def test_get_job_instances_some(self):
        description = ('i-bar', 'foo-0')
        with patch('substrate.describe_instances',
                   return_value=[description], autospec=True) as di_mock:
            ids = get_job_instances('foo')
        self.assertEqual(['i-bar'], [i for i in ids])
        di_mock.assert_called_with(job_name='foo', running=True)

    def test_describe_instances(self):
        with patch('subprocess.check_output',
                   return_value='', autospec=True) as co_mock:
            with patch('substrate.parse_euca', autospec=True) as pe_mock:
                describe_instances(
                    instances=['i-foo'], job_name='bar', running=True)
        co_mock.assert_called_with(
            ['euca-describe-instances',
             '--filter', 'tag:job_name=bar',
             '--filter', 'instance-state-name=running',
             'i-foo'], env=None)
        pe_mock.assert_called_with('')

    def test_parse_euca(self):
        description = parse_euca('')
        self.assertEqual([], [d for d in description])
        euca_data = dedent("""
            header
            INSTANCE\ti-foo\tblah\tbar-0
            INSTANCE\ti-baz\tblah\tbar-1
        """)
        description = parse_euca(euca_data)
        self.assertEqual(
            [('i-foo', 'bar-0'), ('i-baz', 'bar-1')], [d for d in description])

    def test_run_instances_without_series(self):
        euca_data = dedent("""
            header
            INSTANCE\ti-foo\tblah\tbar-0
            INSTANCE\ti-baz\tblah\tbar-1
        """)
        description = [('i-foo', 'bar-0'), ('i-baz', 'bar-1')]
        ami = "ami-atest"
        with patch('subprocess.check_output',
                   return_value=euca_data, autospec=True) as co_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                with patch('substrate.describe_instances',
                           return_value=description, autospec=True) as di_mock:
                    with patch('get_ami.query_ami',
                               return_value=ami, autospec=True) as qa_mock:
                        run_instances(2, 'qux', None)
        co_mock.assert_called_once_with(
            ['euca-run-instances', '-k', 'id_rsa', '-n', '2',
             '-t', 'm1.large', '-g', 'manual-juju-test', ami],
            env=os.environ)
        cc_mock.assert_called_once_with(
            ['euca-create-tags', '--tag', 'job_name=qux', 'i-foo', 'i-baz'],
            env=os.environ)
        di_mock.assert_called_once_with(['i-foo', 'i-baz'], env=os.environ)
        qa_mock.assert_called_once_with('precise', 'amd64')

    @patch('get_ami.query_ami', autospec=True)
    @patch('substrate.describe_instances', autospec=True)
    @patch('subprocess.check_call', autospec=True)
    @patch('subprocess.check_output', autospec=True)
    def test_run_instances_with_series(self,
                                       co_mock, cc_mock, di_mock, qa_mock):
        co_mock.return_value = dedent("""
            header
            INSTANCE\ti-foo\tblah\tbar-0
            INSTANCE\ti-baz\tblah\tbar-1
            """)
        di_mock.return_value = [('i-foo', 'bar-0'), ('i-baz', 'bar-1')]
        qa_mock.return_value = "ami-atest"
        run_instances(2, 'qux', 'wily')
        qa_mock.assert_called_once_with('wily', 'amd64')

    def test_run_instances_tagging_failed(self):
        euca_data = 'INSTANCE\ti-foo\tblah\tbar-0'
        description = [('i-foo', 'bar-0')]
        with patch('subprocess.check_output',
                   return_value=euca_data, autospec=True):
            with patch('subprocess.check_call', autospec=True,
                       side_effect=CalledProcessError('', '')):
                with patch('substrate.describe_instances',
                           return_value=description, autospec=True):
                    with patch('subprocess.call', autospec=True) as c_mock:
                        with self.assertRaises(CalledProcessError):
                            run_instances(1, 'qux', 'trusty')
        c_mock.assert_called_with(['euca-terminate-instances', 'i-foo'])

    def test_run_instances_describe_failed(self):
        euca_data = 'INSTANCE\ti-foo\tblah\tbar-0'
        with patch('subprocess.check_output',
                   return_value=euca_data, autospec=True):
            with patch('substrate.describe_instances',
                       side_effect=CalledProcessError('', '')):
                with patch('subprocess.call', autospec=True) as c_mock:
                    with self.assertRaises(CalledProcessError):
                        run_instances(1, 'qux', 'trusty')
        c_mock.assert_called_with(['euca-terminate-instances', 'i-foo'])

    def test_destroy_job_instances_none(self):
        with patch('substrate.get_job_instances',
                   return_value=[], autospec=True) as gji_mock:
            with patch('subprocess.check_call') as cc_mock:
                destroy_job_instances('foo')
        gji_mock.assert_called_with('foo')
        self.assertEqual(0, cc_mock.call_count)

    def test_destroy_job_instances_some(self):
        with patch('substrate.get_job_instances',
                   return_value=['i-bar'], autospec=True) as gji_mock:
            with patch('subprocess.check_call') as cc_mock:
                destroy_job_instances('foo')
        gji_mock.assert_called_with('foo')
        cc_mock.assert_called_with(['euca-terminate-instances', 'i-bar'])
