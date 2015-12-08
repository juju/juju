from argparse import Namespace
from itertools import count
import os
from unittest import TestCase

from mock import (
    MagicMock,
    patch,
    )
from yaml import safe_dump

from assess_heterogeneous_control import (
    assess_heterogeneous,
    get_clients,
    parse_args,
    test_control_heterogeneous,
    )
from deploy_stack import GET_TOKEN_SCRIPT
from jujupy import (
    _temp_env,
    SimpleEnvironment,
    Status,
    )
from tests.test_deploy_stack import FakeBootstrapManager


__metaclass__ = type


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e'])
        self.assertEqual(args, Namespace(
            initial='a', other='b', base_environment='c',
            environment_name='d', log_dir='e', debug=False,
            upload_tools=False, agent_url=None, agent_stream=None, series=None,
            user=os.environ.get('JENKINS_USER'),
            password=os.environ.get('JENKINS_PASSWORD')))

    def test_parse_args_agent_url(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e', '--agent-url', 'foo',
                           '--user', 'my name', '--password', 'fake pass'])
        self.assertEqual(args.agent_url, 'foo')
        self.assertEqual(args.user, 'my name')
        self.assertEqual(args.password, 'fake pass')

    def test_parse_args_agent_stream(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e',
                           '--agent-stream', 'proposed',
                           '--user', 'my name', '--password', 'fake pass'])
        self.assertEqual(args.agent_stream, 'proposed')
        self.assertEqual(args.user, 'my name')
        self.assertEqual(args.password, 'fake pass')

    def test_parse_args_series(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e', '--series', 'trusty',
                           '--user', 'my name', '--password', 'fake pass'])
        self.assertEqual(args.series, 'trusty')
        self.assertEqual(args.user, 'my name')
        self.assertEqual(args.password, 'fake pass')


class TestGetClients(TestCase):

    def test_get_clients(self):
        boo = {
            ('foo', '--version'): '1.18.73',
            ('bar', '--version'): '1.18.74',
            ('juju', '--version'): '1.18.75',
            ('which', 'juju'): '/usr/bun/juju'
            }
        with _temp_env({'environments': {'baz': {}}}):
            with patch('subprocess.check_output', lambda x: boo[x]):
                initial, other, released = get_clients('foo', 'bar', 'baz',
                                                       True, 'quxx')
        self.assertEqual(initial.env, other.env)
        self.assertEqual(initial.env, released.env)
        self.assertNotIn('tools-metadata-url', initial.env.config)
        self.assertEqual(initial.full_path, os.path.abspath('foo'))
        self.assertEqual(other.full_path, os.path.abspath('bar'))
        self.assertEqual(released.full_path, '/usr/bun/juju')

    def test_get_clients_no_agent(self):
        with _temp_env({'environments': {'baz': {}}}):
            with patch('subprocess.check_output', return_value='1.18.73'):
                initial, other, released = get_clients('foo', 'bar', 'baz',
                                                       True, None)
        self.assertTrue('tools-metadata-url' not in initial.env.config)


class TestAssessHeterogeneous(TestCase):

    @patch('assess_heterogeneous_control.BootstrapManager')
    @patch('assess_heterogeneous_control.test_control_heterogeneous',
           autospec=True)
    @patch('assess_heterogeneous_control.get_clients', autospec=True)
    def test_assess_heterogeneous(self, gc_mock, ch_mock, bm_mock):
        initial = MagicMock()
        gc_mock.return_value = (
            initial, 'other_client', 'released_client')
        assess_heterogeneous(
            'initial', 'other', 'base_env', 'environment_name', 'log_dir',
            False, False, 'agent_url', 'agent_stream', 'series')
        gc_mock.assert_called_once_with(
            'initial', 'other', 'base_env', False, 'agent_url')
        is_jes_enabled = initial.is_jes_enabled.return_value
        bm_mock.assert_called_once_with(
            'environment_name', initial, 'released_client',
            agent_stream='agent_stream', agent_url='agent_url',
            bootstrap_host=None, jes_enabled=is_jes_enabled, keep_env=False,
            log_dir='log_dir', machines=[], permanent=is_jes_enabled,
            region=None, series='series')
        ch_mock.assert_called_once_with(
            bm_mock.return_value, 'other_client', False)


class FakeEnvironmentState:

    def __init__(self):
        self.name = None
        self.machine_id_iter = count()
        self.state_servers = []
        self.services = {}
        self.machines = set()
        self.containers = {}
        self.relations = {}
        self.token = None
        self.exposed = set()

    def add_machine(self):
        machine_id = str(self.machine_id_iter.next())
        self.machines.add(machine_id)
        return machine_id

    def add_container(self, container_type):
        host = self.add_machine()
        container_name = '{}/{}/{}'.format(host, container_type, '0')
        self.containers[host] = {container_name}

    def remove_container(self, container_id):
        for containers in self.containers.values():
            containers.discard(container_id)

    def remove_machine(self, machine_id):
        self.machines.remove(machine_id)
        self.containers.pop(machine_id, None)

    def bootstrap(self, name):
        self.name = name
        self.state_servers.append(self.add_machine())

    def deploy(self, charm_name, service_name):
        self.add_unit(service_name)

    def add_unit(self, service_name):
        machines = self.services.setdefault(service_name, set())
        machines.add(
            ('{}/{}'.format(service_name, str(len(machines))),
             self.add_machine()))

    def remove_unit(self, to_remove):
        for units in self.services.values():
            for unit_id, machine_id in units:
                if unit_id == to_remove:
                    self.remove_machine(machine_id)
                    units.remove((unit_id, machine_id))
                    break

    def destroy_service(self, service_name):
        for unit, machine_id in self.services.pop(service_name):
            self.remove_machine(machine_id)

    def get_status_dict(self):
        machines = dict((mid, {}) for mid in self.machines)
        for host, containers in self.containers.items():
            machines[host]['containers'] = dict((c, {}) for c in containers)
        services = {}
        for service, units in self.services.items():
            unit_map = {}
            for unit_id, machine_id in units:
                unit_map[unit_id] = {'machine': machine_id}
            services[service] = {
                'units': unit_map,
                'relations': self.relations.get(service, {}),
                'exposed': service in self.exposed,
                }
        return {'machines': machines, 'services': services}


class FakeJujuClient:

    def __init__(self):
        self._backing_state = FakeEnvironmentState()
        self.env = SimpleEnvironment('name', {
            'type': 'foo',
            'default-series': 'angsty',
            })
        self.juju_home = 'foo'

    def bootstrap(self, upload_tools):
        self._backing_state.bootstrap(self.env.environment)

    def deploy(self, charm_name, service_name=None):
        if service_name is None:
            service_name = charm_name.split(':')[-1]
        self._backing_state.deploy(charm_name, service_name)

    def juju(self, cmd, args, include_e=True):
        if (cmd, args[:1]) == ('set', ('dummy-source',)):
            name, value = args[1].split('=')
            if name == 'token':
                self._backing_state.token = value
        if cmd == 'deploy':
            self.deploy(*args)
        if cmd == 'destroy-service':
            self._backing_state.destroy_service(*args)
        if cmd == 'add-relation':
            if args[0] == 'dummy-source':
                self._backing_state.relations[args[1]] = {
                        'source': [args[0]]}
        if cmd == 'expose':
            (service,) = args
            self._backing_state.exposed.add(service)
        if cmd == 'add-unit':
            (service,) = args
            self._backing_state.add_unit(service)
        if cmd == 'remove-unit':
            (unit_id,) = args
            self._backing_state.remove_unit(unit_id)
        if cmd == 'add-machine':
            (container_type,) = args
            self._backing_state.add_container(container_type)
        if cmd == 'remove-machine':
            (machine_id,) = args
            if '/' in machine_id:
                self._backing_state.remove_container(machine_id)
            else:
                self._backing_state.remove_machine(machine_id)

    def wait_for_started(self):
        pass

    def get_status(self):
        status_dict = self._backing_state.get_status_dict()
        return Status(status_dict, safe_dump(status_dict))

    def get_juju_output(self, command, *args, **kwargs):
        if (command, args) == ('ssh', ('dummy-sink/0', GET_TOKEN_SCRIPT)):
            return self._backing_state.token
        if (command, args) == ('ssh', ('0', 'lsb_release', '-c')):
            return 'Codename:\t{}\n'.format(self.env.config['default-series'])


class TestTestControlHeterogeneous(TestCase):

    def test_test_control_heterogeneous(self):
        client = FakeJujuClient()
        bs_manager = FakeBootstrapManager(client)
        test_control_heterogeneous(bs_manager, client, True)

    def test_same_home_and_env(self):
        initial_client = FakeJujuClient()
        other_client = FakeJujuClient()
        other_client._backing_state = initial_client._backing_state
        bs_manager = FakeBootstrapManager(initial_client)
        bs_manager.permanent = True
        test_control_heterogeneous(bs_manager, other_client, True)
        self.assertEqual(initial_client.juju_home, other_client.juju_home)
        self.assertEqual(
            initial_client.env.environment, other_client.env.environment)
