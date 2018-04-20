# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2016-2017 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.


from argparse import ArgumentParser
from base64 import b64encode
from contextlib import contextmanager
import copy
from hashlib import sha512
from itertools import count
import json
import logging
import re
import subprocess
import uuid

import pexpect
import yaml

from jujupy import (
    ModelClient,
    JujuData,
)
from jujupy.exceptions import (
    SoftDeadlineExceeded,
)
from jujupy.wait_condition import (
    CommandTime,
    )

__metaclass__ = type

# Python 2 and 3 compatibility
try:
    argtype = basestring
except NameError:
    argtype = str


class ControllerOperation(Exception):

    def __init__(self, operation):
        super(ControllerOperation, self).__init__(
            'Operation "{}" is only valid on controller models.'.format(
                operation))


def assert_juju_call(test_case, mock_method, client, expected_args,
                     call_index=None):
    if call_index is None:
        test_case.assertEqual(len(mock_method.mock_calls), 1)
        call_index = 0
    empty, args, kwargs = mock_method.mock_calls[call_index]
    test_case.assertEqual(args, (expected_args,))


class FakeControllerState:

    def __init__(self):
        self.name = 'name'
        self.state = 'not-bootstrapped'
        self.models = {}
        self.users = {
            'admin': {
                'state': '',
                'permission': 'write'
            }
        }
        self.shares = ['admin']
        self.active_model = None

    def add_model(self, name):
        state = FakeEnvironmentState(self)
        state.name = name
        self.models[name] = state
        state.controller.state = 'created'
        return state

    def require_controller(self, operation, name):
        if name != self.controller_model.name:
            raise ControllerOperation(operation)

    def grant(self, username, permission):
        model_permissions = ['read', 'write', 'admin']
        if permission in model_permissions:
            permission = 'login'
        self.users[username]['access'] = permission

    def add_user_perms(self, username, permissions):
        self.users.update(
            {username: {'state': '', 'permission': permissions}})
        self.shares.append(username)

    def bootstrap(self, model_name, config):
        default_model = self.add_model(model_name)
        default_model.name = model_name
        controller_model = default_model.controller.add_model('controller')
        self.controller_model = controller_model
        controller_model.state_servers.append(controller_model.add_machine())
        self.state = 'bootstrapped'
        default_model.model_config = copy.deepcopy(config)
        self.models[default_model.name] = default_model
        return default_model

    def register(self, name, email, password, twofa):
        self.name = name
        self.add_user_perms('jrandom@external', 'write')
        self.users['jrandom@external'].update(
            {'email': email, 'password': password, '2fa': twofa})
        self.state = 'registered'

    def login_user(self, name, password):
        self.name = name
        self.users.update(
            {name: {'password': password}})

    def destroy(self, kill=False):
        for model in list(self.models.values()):
            model.destroy_model()
        self.models.clear()
        if kill:
            self.state = 'controller-killed'
        else:
            self.state = 'controller-destroyed'


class FakeEnvironmentState:
    """A Fake environment state that can be used by multiple FakeBackends."""

    def __init__(self, controller=None):
        self._clear()
        if controller is not None:
            self.controller = controller
        else:
            self.controller = FakeControllerState()

    def _clear(self):
        self.name = None
        self.machine_id_iter = count()
        self.state_servers = []
        self.services = {}
        self.machines = set()
        self.containers = {}
        self.relations = {}
        self.token = None
        self.exposed = set()
        self.machine_host_names = {}
        self.current_bundle = None
        self.model_config = None
        self.ssh_keys = []

    @property
    def state(self):
        return self.controller.state

    def add_machine(self, host_name=None, machine_id=None):
        if machine_id is None:
            machine_id = str(next(self.machine_id_iter))
        self.machines.add(machine_id)
        if host_name is None:
            host_name = '{}.example.com'.format(machine_id)
        self.machine_host_names[machine_id] = host_name
        return machine_id

    def add_ssh_machines(self, machines):
        for machine in machines:
            self.add_machine()

    def add_container(self, container_type, host=None, container_num=None):
        if host is None:
            host = self.add_machine()
        host_containers = self.containers.setdefault(host, set())
        if container_num is None:
            same_type_containers = [x for x in host_containers if
                                    container_type in x]
            container_num = len(same_type_containers)
        container_name = '{}/{}/{}'.format(host, container_type, container_num)
        host_containers.add(container_name)
        host_name = '{}.example.com'.format(container_name)
        self.machine_host_names[container_name] = host_name

    def remove_container(self, container_id):
        for containers in self.containers.values():
            containers.discard(container_id)

    def remove_machine(self, machine_id, force=False):
        if not force:
            for units, unit_id, loop_machine_id in self.iter_unit_machines():
                if loop_machine_id != machine_id:
                    continue
                logging.error(
                    'no machines were destroyed: machine {} has unit "{}"'
                    ' assigned'.format(machine_id, unit_id))
                raise subprocess.CalledProcessError(1, 'machine assigned.')
        self.machines.remove(machine_id)
        self.containers.pop(machine_id, None)

    def destroy_model(self):
        del self.controller.models[self.name]
        self._clear()
        self.controller.state = 'model-destroyed'

    def _fail_stderr(self, message, returncode=1, cmd='juju', stdout=''):
        exc = subprocess.CalledProcessError(returncode, cmd, stdout)
        exc.stderr = message
        raise exc

    def restore_backup(self):
        self.controller.require_controller('restore', self.name)
        if len(self.state_servers) > 0:
            return self._fail_stderr('Operation not permitted')
        self.state_servers.append(self.add_machine())

    def enable_ha(self):
        self.controller.require_controller('enable-ha', self.name)
        for n in range(2):
            self.state_servers.append(self.add_machine())

    def deploy(self, charm_name, service_name):
        self.add_unit(service_name)

    def deploy_bundle(self, bundle_path):
        self.current_bundle = bundle_path

    def add_unit(self, service_name):
        machines = self.services.setdefault(service_name, set())
        machines.add(
            ('{}/{}'.format(service_name, str(len(machines))),
             self.add_machine()))

    def iter_unit_machines(self):
        for units in self.services.values():
            for unit_id, machine_id in units:
                yield units, unit_id, machine_id

    def remove_unit(self, to_remove):
        for units, unit_id, machine_id in self.iter_unit_machines():
            if unit_id == to_remove:
                units.remove((unit_id, machine_id))
                self.remove_machine(machine_id)
                break
        else:
            raise subprocess.CalledProcessError(
                1, 'juju remove-unit {}'.format(unit_id))

    def destroy_service(self, service_name):
        for unit, machine_id in self.services.pop(service_name):
            self.remove_machine(machine_id)

    def get_status_dict(self):
        machines = {}
        for machine_id in self.machines:
            machine_dict = {
                'juju-status': {'current': 'idle'},
                'series': 'angsty',
                }
            hostname = self.machine_host_names.get(machine_id)
            machine_dict['instance-id'] = machine_id
            if hostname is not None:
                machine_dict['dns-name'] = hostname
            machines[machine_id] = machine_dict
            if machine_id in self.state_servers:
                machine_dict['controller-member-status'] = 'has-vote'
        for host, containers in self.containers.items():
            container_dict = dict((c, {'series': 'angsty'})
                                  for c in containers)
            for container, subdict in container_dict.items():
                subdict.update({'juju-status': {'current': 'idle'}})
                dns_name = self.machine_host_names.get(container)
                if dns_name is not None:
                    subdict['dns-name'] = dns_name

            machines[host]['containers'] = container_dict
        services = {}
        for service, units in self.services.items():
            unit_map = {}
            for unit_id, machine_id in units:
                unit_map[unit_id] = {
                    'machine': machine_id,
                    'juju-status': {'current': 'idle'}}
            services[service] = {
                'units': unit_map,
                'relations': self.relations.get(service, {}),
                'exposed': service in self.exposed,
                }
        return {
            'machines': machines,
            'applications': services,
            'model': {'name': self.name},
            }

    def add_ssh_key(self, keys_to_add):
        errors = []
        for key in keys_to_add:
            if not key.startswith("ssh-rsa "):
                errors.append(
                    'cannot add key "{0}": invalid ssh key: {0}'.format(key))
            elif key in self.ssh_keys:
                errors.append(
                    'cannot add key "{0}": duplicate ssh key: {0}'.format(key))
            else:
                self.ssh_keys.append(key)
        return '\n'.join(errors)

    def remove_ssh_key(self, keys_to_remove):
        errors = []
        for i in reversed(range(len(keys_to_remove))):
            key = keys_to_remove[i]
            if key in ('juju-client-key', 'juju-system-key'):
                keys_to_remove = keys_to_remove[:i] + keys_to_remove[i + 1:]
                errors.append(
                    'cannot remove key id "{0}": may not delete internal key:'
                    ' {0}'.format(key))
        for i in range(len(self.ssh_keys)):
            if self.ssh_keys[i] in keys_to_remove:
                keys_to_remove.remove(self.ssh_keys[i])
                del self.ssh_keys[i]
        errors.extend(
            'cannot remove key id "{0}": invalid ssh key: {0}'.format(key)
            for key in keys_to_remove)
        return '\n'.join(errors)

    def import_ssh_key(self, names_to_add):
        for name in names_to_add:
            self.ssh_keys.append('ssh-rsa FAKE_KEY a key {}'.format(name))
        return ""


class FakeExpectChild:

    def __init__(self, backend, juju_home, extra_env):
        self.backend = backend
        self.juju_home = juju_home
        self.extra_env = extra_env
        self.last_expect = None
        self.exitstatus = None
        self.match = None

    def expect(self, line):
        self.last_expect = line

    def sendline(self, line):
        """Do-nothing implementation of sendline.

        Subclassess will likely override this.
        """

    def close(self):
        self.exitstatus = 0

    def isalive(self):
        return bool(self.exitstatus is not None)


class AutoloadCredentials(FakeExpectChild):

    def __init__(self, backend, juju_home, extra_env):
        super(AutoloadCredentials, self).__init__(backend, juju_home,
                                                  extra_env)
        self.cloud = None

    def sendline(self, line):
        if self.last_expect == (
                '(Select the cloud it belongs to|'
                'Enter cloud to which the credential).* Q to quit.*'):
            self.cloud = line

    def isalive(self):
        juju_data = JujuData('foo', juju_home=self.juju_home)
        juju_data.load_yaml()
        creds = juju_data.credentials.setdefault('credentials', {})
        creds.update({self.cloud: {
            'default-region': self.extra_env['OS_REGION_NAME'],
            self.extra_env['OS_USERNAME']: {
                'domain-name': '',
                'user-domain-name': '',
                'project-domain-name': '',
                'auth-type': 'userpass',
                'username': self.extra_env['OS_USERNAME'],
                'password': self.extra_env['OS_PASSWORD'],
                'tenant-name': self.extra_env['OS_TENANT_NAME'],
                }}})
        juju_data.dump_yaml(self.juju_home)
        return False

    def eof(self):
        return False

    def readline(self):
        return (' 1. openstack region "region" project '
                '"openstack-credentials-0" user "testing-user" (new) '
                ' 2. openstack region "region" project '
                '"openstack-credentials-1" user "testing-user" (new) '
                ' 3. openstack region "region" project '
                '"openstack-credentials-2" user "testing-user" (new) ')


class PromptingExpectChild(FakeExpectChild):
    """A fake ExpectChild based on prompt/response.

    It accepts an iterator of prompts.  If that iterator supports send(),
    the last input to sendline will be sent.

    This allows fairly natural generators, e.g.:

        foo = yield "Please give me foo".

    You can also just iterate through prompts and retrieve the corresponding
    values from self.values at the end.
    """

    def __init__(self, backend, juju_home, extra_env, prompts):
        super(PromptingExpectChild, self).__init__(backend, juju_home,
                                                   extra_env)
        self._prompts = iter(prompts)
        self.values = {}
        self.lines = []
        # If not a generator, invoke next() instead of send.
        self._send = getattr(self._prompts, 'send',
                             lambda x: next(self._prompts))
        self._send_line = None

    @property
    def prompts(self):
        return self._prompts

    def expect(self, pattern):
        if type(pattern) is not list:
            pattern = [pattern]
        try:
            prompt = self._send(self._send_line)
            self._send_line = None
        except StopIteration:
            if pexpect.EOF not in pattern:
                raise
            self.close()
            return
        for regex in pattern:
            if regex is pexpect.EOF:
                continue
            regex_match = re.search(regex, prompt)
            if regex_match is not None:
                self.match = regex_match
                break
        else:
            if pexpect.EOF in pattern:
                raise ValueError('Expected EOF. got "{}"'.format(prompt))
            else:
                raise ValueError(
                    'Regular expression did not match prompt.  Regex: "{}",'
                    ' prompt "{}"'.format(pattern, prompt))
        super(PromptingExpectChild, self).expect(regex)

    def sendline(self, line=''):
        if self._send_line is not None:
            raise ValueError('Sendline called twice with no expect.')
        full_match = self.match.group(0)
        self.values[full_match] = line.rstrip()
        self.lines.append((full_match, line))
        self._send_line = line


class LoginUser(PromptingExpectChild):

    def __init__(self, backend, juju_home, extra_env, username):
        self.username = username
        super(LoginUser, self).__init__(backend, juju_home, extra_env, [
            'Password:',
        ])

    def close(self):
        self.backend.controller_state.login_user(
            self.username,
            self.values['Password'],
            )
        super(LoginUser, self).close()


class RegisterHost(PromptingExpectChild):

    def __init__(self, backend, juju_home, extra_env):
        super(RegisterHost, self).__init__(backend, juju_home, extra_env, [
            'E-Mail:',
            'Password:',
            'Two-factor auth (Enter for none):',
            'Enter a name for this controller:',
        ])

    def close(self):
        self.backend.controller_state.register(
            self.values['Enter a name for this controller:'],
            self.values['E-Mail:'],
            self.values['Password:'],
            self.values['Two-factor auth (Enter for none):'],
            )
        super(RegisterHost, self).close()


class AddCloud(PromptingExpectChild):

    @property
    def provider(self):
        return self.values[self.TYPE]

    @property
    def name_prompt(self):
        return 'Enter a name for your {} cloud:'.format(self.provider)

    REGION_NAME = 'Enter region name:'

    TYPE = 'Select cloud type:'

    AUTH = 'Select one or more auth types separated by commas:'

    API_ENDPOINT = 'Enter the API endpoint url:'

    CLOUD_ENDPOINT = 'Enter the API endpoint url for the cloud:'

    REGION_ENDPOINT = (
        'Enter the API endpoint url for the region [use cloud api url]:')

    HOST = "Enter the controller's hostname or IP address:"

    ANOTHER_REGION = 'Enter another region? (Y/n):'

    VCENTER_ADDRESS = "Enter the vCenter address or URL:"

    DATACENTER_NAME = "Enter datacenter name:"

    ANOTHER_DATACENTER = 'Enter another datacenter? (Y/n):'

    def cant_validate(self, endpoint):
        if self.provider in ('openstack', 'maas'):
            if self.provider == 'openstack':
                server_type = 'Openstack'
                reprompt = self.CLOUD_ENDPOINT
            else:
                server_type = 'MAAS'
                reprompt = self.API_ENDPOINT
            msg = 'No {} server running at {}'.format(server_type, endpoint)
        elif self.provider == 'manual':
            msg = 'ssh: Could not resolve hostname {}'.format(endpoint)
            reprompt = self.HOST
        elif self.provider == 'vsphere':
            msg = '{}: invalid domain name'.format(endpoint)
            reprompt = self.VCENTER_ADDRESS
        return "Can't validate endpoint: {}\n{}".format(
            msg, reprompt)

    def __init__(self, backend, juju_home, extra_env):
        super(AddCloud, self).__init__(
            backend, juju_home, extra_env, self.iter_prompts())

    def iter_prompts(self):
        while True:
            provider_type = yield self.TYPE
            if provider_type != 'bogus':
                break
        while True:
            name = yield self.name_prompt
            if '/' not in name:
                break
        if provider_type == 'maas':
            endpoint = yield self.API_ENDPOINT
            while len(endpoint) > 1000:
                yield self.cant_validate(endpoint)
        elif provider_type == 'manual':
            endpoint = yield self.HOST
            while len(endpoint) > 1000:
                yield self.cant_validate(endpoint)
        elif provider_type == 'openstack':
            endpoint = yield self.CLOUD_ENDPOINT
            while len(endpoint) > 1000:
                yield self.cant_validate(endpoint)
            while True:
                auth = yield self.AUTH
                if 'invalid' not in auth:
                    break
            while True:
                yield self.REGION_NAME
                endpoint = yield self.REGION_ENDPOINT
                if len(endpoint) > 1000:
                    yield self.cant_validate(endpoint)
                if (yield self.ANOTHER_REGION) == 'n':
                    break
        elif provider_type == 'vsphere':
            endpoint = yield self.VCENTER_ADDRESS
            if len(endpoint) > 1000:
                yield self.cant_validate(endpoint)
            while True:
                yield self.DATACENTER_NAME
                if (yield self.ANOTHER_DATACENTER) == 'n':
                    break

    def close(self):
        cloud = {
            'type': self.values[self.TYPE],
        }
        if cloud['type'] == 'maas':
            cloud.update({'endpoint': self.values[self.API_ENDPOINT]})
        if cloud['type'] == 'manual':
            cloud.update({'endpoint': self.values[self.HOST]})
        if cloud['type'] == 'openstack':
            regions = {}
            for match, line in self.lines:
                if match == self.REGION_NAME:
                    cur_region = {}
                    regions[line] = cur_region
                if match == self.REGION_ENDPOINT:
                    cur_region['endpoint'] = line
            cloud.update({
                'endpoint': self.values[self.CLOUD_ENDPOINT],
                'auth-types': self.values[self.AUTH].split(','),
                'regions': regions
                })
        if cloud['type'] == 'vsphere':
            regions = {}
            for match, line in self.lines:
                if match == self.DATACENTER_NAME:
                    cur_region = {}
                    regions[line] = cur_region
            cloud.update({
                'endpoint': self.values[self.VCENTER_ADDRESS],
                'regions': regions,
                })
        self.backend.clouds[self.values[self.name_prompt]] = cloud


class AddCloud2_1(AddCloud):

    REGION_ENDPOINT = 'Enter the API endpoint url for the region:'

    VCENTER_ADDRESS = AddCloud.CLOUD_ENDPOINT

    DATACENTER_NAME = AddCloud.REGION_NAME

    ANOTHER_DATACENTER = AddCloud.ANOTHER_REGION


class FakeBackend:
    """A fake juju backend for tests.

    This is a partial implementation, but should be suitable for many uses,
    and can be extended.

    The state is provided by controller_state, so that multiple clients and
    backends can manipulate the same state.
    """

    def __init__(self, controller_state, feature_flags=None, version=None,
                 full_path=None, debug=False, past_deadline=False):
        assert isinstance(controller_state, FakeControllerState)
        self.controller_state = controller_state
        if feature_flags is None:
            feature_flags = set()
        self.feature_flags = feature_flags
        self.version = version
        self.full_path = full_path
        self.debug = debug
        self.juju_timings = {}
        self.log = logging.getLogger('jujupy')
        self._past_deadline = past_deadline
        self._ignore_soft_deadline = False
        self.clouds = {}
        self.action_results = {}
        self.action_queue = {}
        self.added_models = []

    def track_model(self, client):
        pass

    def untrack_model(self, client):
        pass

    def clone(self, full_path=None, version=None, debug=None,
              feature_flags=None):
        if version is None:
            version = self.version
        if full_path is None:
            full_path = self.full_path
        if debug is None:
            debug = self.debug
        if feature_flags is None:
            feature_flags = set(self.feature_flags)
        controller_state = self.controller_state
        return self.__class__(controller_state, feature_flags, version,
                              full_path, debug,
                              past_deadline=self._past_deadline)

    def is_feature_enabled(self, feature):
        return bool(feature in self.feature_flags)

    @contextmanager
    def ignore_soft_deadline(self):
        """Ignore the client deadline.  For cleanup code."""
        old_val = self._ignore_soft_deadline
        self._ignore_soft_deadline = True
        try:
            yield
        finally:
            self._ignore_soft_deadline = old_val

    @contextmanager
    def _check_timeouts(self):
        try:
            yield
        finally:
            if self._past_deadline and not self._ignore_soft_deadline:
                raise SoftDeadlineExceeded()

    def get_active_model(self, juju_home):
        return self.controller_state.active_model

    def get_active_controller(self, juju_home):
        return self.controller_state.name

    def deploy(self, model_state, charm_name, num, service_name=None,
               series=None):
        if service_name is None:
            service_name = charm_name.split(':')[-1].split('/')[-1]
        for i in range(num):
            model_state.deploy(charm_name, service_name)

    def bootstrap(self, args):
        parser = ArgumentParser()
        parser.add_argument('cloud_name_region')
        parser.add_argument('controller_name')
        parser.add_argument('--constraints')
        parser.add_argument('--config')
        parser.add_argument('--default-model')
        parser.add_argument('--agent-version')
        parser.add_argument('--bootstrap-series')
        parser.add_argument('--upload-tools', action='store_true')
        parsed = parser.parse_args(args)
        with open(parsed.config) as config_file:
            config = yaml.safe_load(config_file)
        cloud_region = parsed.cloud_name_region.split('/', 1)
        cloud = cloud_region[0]
        # Although they are specified with specific arguments instead of as
        # config, these values are listed by model-config:
        # name, region, type (from cloud).
        config['type'] = cloud
        if len(cloud_region) > 1:
            config['region'] = cloud_region[1]
        config['name'] = parsed.default_model
        if parsed.bootstrap_series is not None:
            config['default-series'] = parsed.bootstrap_series
        self.controller_state.bootstrap(parsed.default_model, config)

    def quickstart(self, model_name, config, bundle):
        default_model = self.controller_state.bootstrap(model_name, config)
        default_model.deploy_bundle(bundle)

    def add_machines(self, model_state, args):
        if len(args) == 0:
            return model_state.add_machine()
        ssh_machines = [a[4:] for a in args if a.startswith('ssh:')]
        if len(ssh_machines) == len(args):
            return model_state.add_ssh_machines(ssh_machines)
        parser = ArgumentParser()
        parser.add_argument('host_placement', nargs='*')
        parser.add_argument('-n', type=int, dest='count', default='1')
        parser.add_argument('--series')
        parsed = parser.parse_args(args)
        if len(parsed.host_placement) > 0 and parsed.count != 1:
                raise subprocess.CalledProcessError(
                    1, 'cannot use -n when specifying a placement directive.'
                    'See Lp #1384350.')
        if len(parsed.host_placement) == 1:
            split = parsed.host_placement[0].split(':')
            if len(split) == 1:
                container_type = split[0]
                host = None
            else:
                container_type, host = split
            for x in range(parsed.count):
                model_state.add_container(container_type, host=host)
        else:
            for x in range(parsed.count):
                model_state.add_machine()

    def get_controller_model_name(self):
        return self.controller_state.controller_model.name

    def make_controller_dict(self, controller_name):
        controller_model = self.controller_state.controller_model
        server_id = list(controller_model.state_servers)[0]
        server_hostname = controller_model.machine_host_names[server_id]
        api_endpoint = '{}:23'.format(server_hostname)
        uuid = 'b74b0e9a-81cb-4161-8396-bd5149e2a3cc'
        return {
            controller_name: {
                'details': {
                    'api-endpoints': [api_endpoint],
                    'uuid': uuid,
                }
            }
        }

    def list_models(self):
        model_names = [state.name for state in
                       self.controller_state.models.values()]
        return {'models': [{'name': n} for n in model_names]}

    def list_users(self):
        user_names = [name for name in
                      self.controller_state.users.keys()]
        user_list = []
        for n in user_names:
            if n == 'admin':
                append_dict = {'access': 'superuser', 'user-name': n,
                               'display-name': n}
            else:
                access = self.controller_state.users[n]['access']
                append_dict = {
                    'access': access, 'user-name': n}
            user_list.append(append_dict)
        return user_list

    def show_user(self, user_name):
        if user_name is None:
            raise Exception("No user specified")
        if user_name == 'admin':
            user_status = {'access': 'superuser', 'user-name': user_name,
                           'display-name': user_name}
        else:
            user_status = {'user-name': user_name, 'display-name': ''}
        return user_status

    def get_users(self):
        share_names = self.controller_state.shares
        permissions = []
        for key, value in self.controller_state.users.iteritems():
            if key in share_names:
                permissions.append(value['permission'])
        share_list = {}
        for i, (share_name, permission) in enumerate(
                zip(share_names, permissions)):
            share_list[share_name] = {'display-name': share_name,
                                      'access': permission}
            if share_name != 'admin':
                share_list[share_name].pop('display-name')
            else:
                share_list[share_name]['access'] = 'admin'
        return share_list

    def show_model(self):
        # To get data from the model we would need:
        # self.controller_state.current_model
        model_name = 'name'
        data = {
            'name': model_name,
            'owner': 'admin',
            'life': 'alive',
            'status': {'current': 'available', 'since': '15 minutes ago'},
            'users': self.get_users(),
            }
        return {model_name: data}

    def run_action(self, unit_id, action):
        action_uuid = '{}'.format(uuid.uuid1())
        try:
            result = self.action_results[unit_id][action]
            self.action_queue[action_uuid] = result
        except KeyError:
            raise ValueError('No such action "{0}"'
                             ' specified for unit {1}.'.format(action,
                                                               unit_id))
        return ('Action queued with id: {}'.format(action_uuid))

    def show_action_output(self, action_uuid):
        return self.action_queue.get(action_uuid, None)

    def _log_command(self, command, args, model, level=logging.INFO):
        full_args = ['juju', command]
        if model is not None:
            full_args.extend(['-m', model])
        full_args.extend(args)
        self.log.log(level, u' '.join(full_args))

    def juju(self, command, args, used_feature_flags,
             juju_home, model=None, check=True, timeout=None, extra_env=None,
             suppress_err=False):
        if 'service' in command:
            raise Exception('Command names must not contain "service".')

        if isinstance(args, argtype):
            args = (args,)
        self._log_command(command, args, model)
        if model is not None:
            if ':' in model:
                model = model.split(':')[1]
            model_state = self.controller_state.models[model]
            if ((command, args[:1]) == ('set-config', ('dummy-source',)) or
                    (command, args[:1]) == ('config', ('dummy-source',))):
                name, value = args[1].split('=')
                if name == 'token':
                    model_state.token = value
            if command == 'deploy':
                parser = ArgumentParser()
                parser.add_argument('charm_name')
                parser.add_argument('service_name', nargs='?')
                parser.add_argument('--to')
                parser.add_argument('--series')
                parser.add_argument('-n')
                parsed = parser.parse_args(args)
                num = int(parsed.n or 1)
                self.deploy(model_state, parsed.charm_name, num,
                            parsed.service_name, parsed.series)
                return (0, CommandTime(command, args))
            if command == 'remove-application':
                model_state.destroy_service(*args)
            if command == 'add-relation':
                if args[0] == 'dummy-source':
                    model_state.relations[args[1]] = {'source': [args[0]]}
            if command == 'expose':
                (service,) = args
                model_state.exposed.add(service)
            if command == 'unexpose':
                (service,) = args
                model_state.exposed.remove(service)
            if command == 'add-unit':
                (service,) = args
                model_state.add_unit(service)
            if command == 'remove-unit':
                (unit_id,) = args
                model_state.remove_unit(unit_id)
            if command == 'add-machine':
                return self.add_machines(model_state, args)
            if command == 'remove-machine':
                parser = ArgumentParser()
                parser.add_argument('machine_id')
                parser.add_argument('--force', action='store_true')
                parsed = parser.parse_args(args)
                machine_id = parsed.machine_id
                if '/' in machine_id:
                    model_state.remove_container(machine_id)
                else:
                    model_state.remove_machine(machine_id, parsed.force)
            if command == 'quickstart':
                parser = ArgumentParser()
                parser.add_argument('--constraints')
                parser.add_argument('--no-browser', action='store_true')
                parser.add_argument('bundle')
                parsed = parser.parse_args(args)
                # Released quickstart doesn't seem to provide the config via
                # the commandline.
                self.quickstart(model, {}, parsed.bundle)
        else:
            if command == 'bootstrap':
                self.bootstrap(args)
            if command == 'destroy-controller':
                if self.controller_state.state not in ('bootstrapped',
                                                       'created'):
                    raise subprocess.CalledProcessError(1, 'Not bootstrapped.')
                self.controller_state.destroy()
            if command == 'kill-controller':
                if self.controller_state.state == 'not-bootstrapped':
                    return (0, CommandTime(command, args))
                self.controller_state.destroy(kill=True)
                return (0, CommandTime(command, args))
            if command == 'destroy-model':
                model = args[0].split(':')[1]
                try:
                    model_state = self.controller_state.models[model]
                except KeyError:
                    raise subprocess.CalledProcessError(1, 'No such model')
                model_state.destroy_model()
            if command == 'enable-ha':
                parser = ArgumentParser()
                parser.add_argument('-n', '--number')
                parser.add_argument('-c', '--controller')
                parsed = parser.parse_args(args)
                if not self.controller_state.name == parsed.controller:
                    raise AssertionError('Test does not setup controller name')
                model_state = self.controller_state.controller_model
                model_state.enable_ha()
            if command == 'add-model':
                parser = ArgumentParser()
                parser.add_argument('-c', '--controller')
                parser.add_argument('--config')
                parser.add_argument('--credential')
                parser.add_argument('model_name')
                parser.add_argument('cloud-region', nargs='?')
                parsed = parser.parse_args(args)
                model_client = self.controller_state.add_model(
                    parsed.model_name)
                self.added_models.append(model_client)
            if command == 'revoke':
                user_name = args[2]
                permissions = args[3]
                per = self.controller_state.users[user_name]['permission']
                if per == permissions:
                    if permissions == 'read':
                        self.controller_state.shares.remove(user_name)
                        per = ''
                    else:
                        per = 'read'
            if command == 'grant':
                username = args[0]
                permission = args[1]
                self.controller_state.grant(username, permission)
            if command == 'remove-user':
                username = args[0]
                self.controller_state.users.pop(username)
                if username in self.controller_state.shares:
                    self.controller_state.shares.remove(username)
            if command == 'restore-backup':
                model_state.restore_backup()
            return 0, CommandTime(command, args)

    @contextmanager
    def juju_async(self, command, args, used_feature_flags,
                   juju_home, model=None, timeout=None):
        yield
        self.juju(command, args, used_feature_flags,
                  juju_home, model, timeout=timeout)

    def get_juju_output(self, command, args, used_feature_flags, juju_home,
                        model=None, timeout=None, user_name=None,
                        merge_stderr=False):
        if 'service' in command:
            raise Exception('No service')
        with self._check_timeouts():
            self._log_command(command, args, model, logging.DEBUG)
            if model is not None:
                if ':' in model:
                    model = model.split(':')[1]
                model_state = self.controller_state.models[model]
            sink_cat = ('dummy-sink/0', 'cat', '/var/run/dummy-sink/token')
            if (command, args) == ('ssh', sink_cat):
                return model_state.token
            if (command, args) == ('ssh', ('0', 'lsb_release', '-c')):
                return 'Codename:\t{}\n'.format(
                    model_state.model_config['default-series'])
            if command in ('model-config', 'get-model-config'):
                return yaml.safe_dump(model_state.model_config)
            if command == 'show-controller':
                return yaml.safe_dump(self.make_controller_dict(args[0]))
            if command == 'list-models':
                return yaml.safe_dump(self.list_models())
            if command == 'list-users':
                return json.dumps(self.list_users())
            if command == 'show-model':
                return json.dumps(self.show_model())
            if command == 'show-user':
                return json.dumps(self.show_user(user_name))
            if command == 'add-user':
                permissions = 'read'
                if set(["--acl", "write"]).issubset(args):
                    permissions = 'write'
                username = args[0]
                info_string = 'User "{}" added\n'.format(username)
                self.controller_state.add_user_perms(username, permissions)
                register_string = get_user_register_command_info(username)
                return info_string + register_string
            if command == 'show-status':
                status_dict = model_state.get_status_dict()
                # Parsing JSON is much faster than parsing YAML, and JSON is a
                # subset of YAML, so emit JSON.
                return json.dumps(status_dict).encode('utf-8')
            if command == 'create-backup':
                self.controller_state.require_controller('backup', model)
                return 'juju-backup-0.tar.gz'
            if command == 'ssh-keys':
                lines = ['Keys used in model: ' + model_state.name]
                if '--full' in args:
                    lines.extend(model_state.ssh_keys)
                else:
                    lines.extend(':fake:fingerprint: ({})'.format(
                        k.split(' ', 2)[-1]) for k in model_state.ssh_keys)
                return '\n'.join(lines)
            if command == 'add-ssh-key':
                return model_state.add_ssh_key(args)
            if command == 'remove-ssh-key':
                return model_state.remove_ssh_key(args)
            if command == 'import-ssh-key':
                return model_state.import_ssh_key(args)
            if command == 'run-action':
                unit_id = args[0]
                action = args[1]
                return self.run_action(unit_id, action)
            if command == 'show-action-output':
                return self.show_action_output(args[0])
            return ''

    def expect(self, command, args, used_feature_flags, juju_home, model=None,
               timeout=None, extra_env=None):
        if command == 'autoload-credentials':
            return AutoloadCredentials(self, juju_home, extra_env)
        if command == 'register':
            return RegisterHost(self, juju_home, extra_env)
        if command == 'add-cloud':
            return AddCloud(self, juju_home, extra_env)
        if command == 'login -u':
            return LoginUser(self, juju_home, extra_env, args[0])
        return FakeExpectChild(self, juju_home, extra_env)

    def pause(self, seconds):
        pass


def get_user_register_command_info(username):
    code = get_user_register_token(username)
    return 'Please send this command to {}\n    juju register {}'.format(
        username, code)


def get_user_register_token(username):
    return b64encode(sha512(username.encode('utf-8')).digest()).decode('ascii')


def fake_juju_client(env=None, full_path=None, debug=False, version='2.0.0',
                     _backend=None, cls=ModelClient, juju_home=None):
    if juju_home is None:
        if env is None or env.juju_home is None:
            juju_home = 'foo'
        else:
            juju_home = env.juju_home
    if env is None:
        env = JujuData('name', {
            'type': 'foo',
            'default-series': 'angsty',
            'region': 'bar',
            }, juju_home=juju_home)
        env.credentials = {'credentials': {'foo': {'creds': {}}}}
    if _backend is None:
        backend_state = FakeControllerState()
        _backend = FakeBackend(
            backend_state, version=version, full_path=full_path,
            debug=debug)
    client = cls(
        env, version, full_path, juju_home, debug, _backend=_backend)
    client.bootstrap_replaces = {}
    return client
