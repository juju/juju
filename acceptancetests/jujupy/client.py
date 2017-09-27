# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2017 Canonical Ltd.
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


from __future__ import print_function

from collections import (
    defaultdict,
    namedtuple,
    )
from contextlib import (
    contextmanager,
    )
from copy import deepcopy
from datetime import (
    datetime,
    timedelta,
    )
import errno
from itertools import chain
import json
from locale import getpreferredencoding
import logging
import os
import re
import shutil
import subprocess
import sys
import time

from dateutil.parser import parse as datetime_parse
from dateutil import tz
import pexpect
import yaml

from jujupy.configuration import (
    get_bootstrap_config_path,
    get_environments_path,
    get_jenv_path,
    get_juju_home,
    get_selected_environment,
    )
from jujupy.utility import (
    check_free_disk_space,
    ensure_dir,
    get_timeout_path,
    is_ipv6_address,
    JujuResourceTimeout,
    pause,
    quote,
    qualified_model_name,
    scoped_environ,
    skip_on_missing_file,
    split_address_port,
    temp_dir,
    temp_yaml_file,
    unqualified_model_name,
    until_timeout,
    )


__metaclass__ = type

# Python 2 and 3 compatibility
try:
    argtype = basestring
except NameError:
    argtype = str

AGENTS_READY = set(['started', 'idle'])
WIN_JUJU_CMD = os.path.join('\\', 'Progra~2', 'Juju', 'juju.exe')

JUJU_DEV_FEATURE_FLAGS = 'JUJU_DEV_FEATURE_FLAGS'
CONTROLLER = 'controller'
KILL_CONTROLLER = 'kill-controller'
SYSTEM = 'system'

KVM_MACHINE = 'kvm'
LXC_MACHINE = 'lxc'
LXD_MACHINE = 'lxd'

_DEFAULT_BUNDLE_TIMEOUT = 3600

_jes_cmds = {KILL_CONTROLLER: {
    'create': 'create-environment',
    'kill': KILL_CONTROLLER,
}}
for super_cmd in [SYSTEM, CONTROLLER]:
    _jes_cmds[super_cmd] = {
        'create': '{} create-environment'.format(super_cmd),
        'kill': '{} kill'.format(super_cmd),
    }

log = logging.getLogger("jujupy")


class StatusTimeout(Exception):
    """Raised when 'juju status' timed out."""


class SoftDeadlineExceeded(Exception):
    """Raised when an overall client operation takes too long."""

    def __init__(self):
        super(SoftDeadlineExceeded, self).__init__(
            'Operation exceeded deadline.')


class NoProvider(Exception):
    """Raised when an environment defines no provider."""


class TypeNotAccepted(Exception):
    """Raised when the provided type was not accepted."""


class NameNotAccepted(Exception):
    """Raised when the provided name was not accepted."""


class InvalidEndpoint(Exception):
    """Raised when the provided endpoint was deemed invalid."""


class AuthNotAccepted(Exception):
    """Raised when the provided auth was not accepted."""


class NoActiveModel(Exception):
    """Raised when no active model could be found."""


class NoActiveControllers(Exception):
    """Raised when no active environment could be found."""


def get_timeout_prefix(duration, timeout_path=None):
    """Return extra arguments to run a command with a timeout."""
    if timeout_path is None:
        timeout_path = get_timeout_path()
    return (sys.executable, timeout_path, '%.2f' % duration, '--')


def get_teardown_timeout(client):
    """Return the timeout need byt the client to teardown resources."""
    if client.env.provider == 'azure':
        return 2700
    elif client.env.provider == 'gce':
        return 1200
    else:
        return 600


def parse_new_state_server_from_error(error):
    err_str = str(error)
    output = getattr(error, 'output', None)
    if output is not None:
        err_str += output
    matches = re.findall(r'Attempting to connect to (.*):22', err_str)
    if matches:
        return matches[-1]
    return None


class ErroredUnit(Exception):

    def __init__(self, unit_name, state):
        msg = '%s is in state %s' % (unit_name, state)
        Exception.__init__(self, msg)
        self.unit_name = unit_name
        self.state = state


class UpgradeMongoNotSupported(Exception):

    def __init__(self):
        super(UpgradeMongoNotSupported, self).__init__(
            'This client does not support upgrade-mongo')


class JESNotSupported(Exception):

    def __init__(self):
        super(JESNotSupported, self).__init__(
            'This client does not support JES')


class JESByDefault(Exception):

    def __init__(self):
        super(JESByDefault, self).__init__(
            'This client does not need to enable JES')


Machine = namedtuple('Machine', ['machine_id', 'info'])


def coalesce_agent_status(agent_item):
    """Return the machine agent-state or the unit agent-status."""
    state = agent_item.get('agent-state')
    if state is None and agent_item.get('agent-status') is not None:
        state = agent_item.get('agent-status').get('current')
    if state is None and agent_item.get('juju-status') is not None:
        state = agent_item.get('juju-status').get('current')
    if state is None:
        state = 'no-agent'
    return state


class CannotConnectEnv(subprocess.CalledProcessError):

    def __init__(self, e):
        super(CannotConnectEnv, self).__init__(e.returncode, e.cmd, e.output)


class StatusNotMet(Exception):

    _fmt = 'Expected status not reached in {env}.'

    def __init__(self, environment_name, status):
        self.env = environment_name
        self.status = status

    def __str__(self):
        return self._fmt.format(env=self.env)


class AgentsNotStarted(StatusNotMet):

    _fmt = 'Timed out waiting for agents to start in {env}.'


class VersionsNotUpdated(StatusNotMet):

    _fmt = 'Some versions did not update.'


class WorkloadsNotReady(StatusNotMet):

    _fmt = 'Workloads not ready in {env}.'


class ApplicationsNotStarted(StatusNotMet):

    _fmt = 'Timed out waiting for applications to start in {env}.'


class VotingNotEnabled(StatusNotMet):

    _fmt = 'Timed out waiting for voting to be enabled in {env}.'


class SimpleEnvironment:
    """Represents a model in a JUJU_HOME directory for juju 1."""

    def __init__(self, environment, config=None, juju_home=None,
                 controller=None, bootstrap_to=None):
        """Constructor.

        :param environment: Name of the environment.
        :param config: Dictionary with configuration options, default is None.
        :param juju_home: Path to JUJU_HOME directory, default is None.
        :param controller: Controller instance-- this model's controller.
            If not given or None a new instance is created.
        :param bootstrap_to: A placement directive to use when bootstrapping.
            See Juju provider docs to examples of what Juju might expect.
        """
        self.user_name = None
        if controller is None:
            controller = Controller(environment)
        self.controller = controller
        self.environment = environment
        self._config = config
        self.juju_home = juju_home
        self.bootstrap_to = bootstrap_to
        if self._config is not None:
            try:
                provider = self.provider
            except NoProvider:
                provider = None
            self.local = bool(provider == 'local')
            self.kvm = (
                self.local and bool(self._config.get('container') == 'kvm'))
            self.maas = bool(provider == 'maas')
            self.joyent = bool(provider == 'joyent')
        else:
            self.local = False
            self.kvm = False
            self.maas = False
            self.joyent = False

    def get_option(self, key, default=None):
        return self._config.get(key, default)

    def update_config(self, new_config):
        for key, value in new_config.items():
            if key == 'region':
                logging.warning(
                    'Using set_region to set region to "{}".'.format(value))
                self.set_region(value)
                continue
            if key == 'type':
                logging.warning('Setting type is not 2.x compatible.')
            self._config[key] = value

    def discard_option(self, key):
        return self._config.pop(key, None)

    def make_config_copy(self):
        return deepcopy(self._config)

    @property
    def provider(self):
        """Return the provider type for this environment.

        See get_cloud to determine the specific cloud.
        """
        try:
            return self._config['type']
        except KeyError:
            raise NoProvider('No provider specified.')

    def is_cloud_provider(self):
        """Return True if the commandline cloud is a provider.

        This is True when LXD or Manual would be specified on the commandline,
        false otherwse.
        """
        return bool(self.provider in ('lxd', 'manual'))

    def get_region(self):
        """Determine the region from a 1.x-style config.

        This requires translating Azure's and Joyent's conventions for
        specifying region.

        It means that endpoint, rather than region, should be supplied if the
        cloud (not the provider) is named "lxd" or "manual".

        May return None for MAAS or LXD clouds.
        """
        provider = self.provider
        # In 1.x, providers define region differently.  Translate.
        if provider == 'azure':
            if 'tenant-id' not in self._config:
                return self._config['location'].replace(' ', '').lower()
            return self._config['location']
        elif provider == 'joyent':
            matcher = re.compile('https://(.*).api.joyentcloud.com')
            return matcher.match(self._config['sdc-url']).group(1)
        elif provider == 'maas':
            return None
        # In 2.x, certain providers can be specified on the commandline in
        # place of a cloud.  The "region" in these cases is the endpoint.
        elif self.is_cloud_provider():
            return self._get_config_endpoint()
        else:
            # The manual provider is typically used without a region.
            if provider == 'manual':
                return self._config.get('region')
            return self._config['region']

    def _get_config_endpoint(self):
        if self.provider == 'lxd':
            return self._config.get('region', 'localhost')
        elif self.provider == 'manual':
            return self._config['bootstrap-host']

    def set_region(self, region):
        """Assign the region to a 1.x-style config.

        This requires translating Azure's and Joyent's conventions for
        specifying region.

        It means that endpoint, rather than region, should be updated if the
        cloud (not the provider) is named "lxd" or "manual".

        Only None is acccepted for MAAS.
        """
        try:
            provider = self.provider
            cloud_is_provider = self.is_cloud_provider()
        except NoProvider:
            provider = None
            cloud_is_provider = False
        if provider == 'azure':
            self._config['location'] = region
        elif provider == 'joyent':
            self._config['sdc-url'] = (
                'https://{}.api.joyentcloud.com'.format(region))
        elif cloud_is_provider:
            self._set_config_endpoint(region)
        elif provider == 'maas':
            if region is not None:
                raise ValueError('Only None allowed for maas.')
        else:
            self._config['region'] = region

    def _set_config_endpoint(self, endpoint):
        if self.provider == 'lxd':
            self._config['region'] = endpoint
        elif self.provider == 'manual':
            self._config['bootstrap-host'] = endpoint

    def clone(self, model_name=None):
        config = deepcopy(self._config)
        if model_name is None:
            model_name = self.environment
        else:
            config['name'] = unqualified_model_name(model_name)
        result = self.__class__(model_name, config, juju_home=self.juju_home,
                                controller=self.controller,
                                bootstrap_to=self.bootstrap_to)
        result.local = self.local
        result.kvm = self.kvm
        result.maas = self.maas
        result.joyent = self.joyent
        result.user_name = self.user_name
        return result

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        if self.environment != other.environment:
            return False
        if self._config != other._config:
            return False
        if self.local != other.local:
            return False
        if self.maas != other.maas:
            return False
        if self.bootstrap_to != other.bootstrap_to:
            return False
        return True

    def __ne__(self, other):
        return not self == other

    def set_model_name(self, model_name, set_controller=True):
        if set_controller:
            self.controller.name = model_name
        self.environment = model_name
        self._config['name'] = unqualified_model_name(model_name)

    @classmethod
    def from_config(cls, name):
        """Create an environment from the configuation file.

        :param name: Name of the environment to get the configuration from."""
        return cls._from_config(name)

    @classmethod
    def _from_config(cls, name):
        config, selected = get_selected_environment(name)
        if name is None:
            name = selected
        return cls(name, config)

    @contextmanager
    def make_jes_home(self, juju_home, dir_name, new_config):
        """Make a JUJU_HOME/DATA directory to avoid conflicts.

        :param juju_home: Current JUJU_HOME/DATA directory, used as a
            base path for the new directory.
        :param dir_name: Name of sub-directory to make the home in.
        :param new_config: Dictionary representing the contents of
            the environments.yaml configuation file."""
        home_path = jes_home_path(juju_home, dir_name)
        with skip_on_missing_file():
            shutil.rmtree(home_path)
        os.makedirs(home_path)
        self.dump_yaml(home_path, new_config)
        # For extention: Add all files carried over to the list.
        for file_name in ['public-clouds.yaml']:
            src_path = os.path.join(juju_home, file_name)
            with skip_on_missing_file():
                shutil.copy(src_path, home_path)
        yield home_path

    def get_cloud_credentials(self):
        """Return the credentials for this model's cloud.

        This implementation returns config variables in addition to
        credentials.
        """
        return self._config

    def dump_yaml(self, path, config):
        dump_environments_yaml(path, config)


class JujuData(SimpleEnvironment):
    """Represents a model in a JUJU_DATA directory for juju 2."""

    def __init__(self, environment, config=None, juju_home=None,
                 controller=None, cloud_name=None, bootstrap_to=None):
        """Constructor.

        This extends SimpleEnvironment's constructor.

        :param environment: Name of the environment.
        :param config: Dictionary with configuration options; default is None.
        :param juju_home: Path to JUJU_DATA directory. If None (the default),
            the home directory is autodetected.
        :param controller: Controller instance-- this model's controller.
            If not given or None, a new instance is created.
        :param bootstrap_to: A placement directive to use when bootstrapping.
            See Juju provider docs to examples of what Juju might expect.
        """
        if juju_home is None:
            juju_home = get_juju_home()
        super(JujuData, self).__init__(environment, config, juju_home,
                                       controller, bootstrap_to)
        self.credentials = {}
        self.clouds = {}
        self._cloud_name = cloud_name

    def clone(self, model_name=None):
        result = super(JujuData, self).clone(model_name)
        result.credentials = deepcopy(self.credentials)
        result.clouds = deepcopy(self.clouds)
        result._cloud_name = self._cloud_name
        return result

    @classmethod
    def from_env(cls, env):
        juju_data = cls(env.environment, env._config, env.juju_home)
        juju_data.load_yaml()
        return juju_data

    def update_config(self, new_config):
        if 'type' in new_config:
            raise ValueError('type cannot be set via update_config.')
        if self._cloud_name is not None:
            # Do not accept changes that would alter the computed cloud name
            # if computed cloud names are not in use.
            for endpoint_key in ['maas-server', 'auth-url', 'host']:
                if endpoint_key in new_config:
                    raise ValueError(
                        '{} cannot be changed with explicit cloud'
                        ' name.'.format(endpoint_key))
        super(JujuData, self).update_config(new_config)

    def load_yaml(self):
        try:
            with open(os.path.join(self.juju_home, 'credentials.yaml')) as f:
                self.credentials = yaml.safe_load(f)
        except IOError as e:
            if e.errno != errno.ENOENT:
                raise RuntimeError(
                    'Failed to read credentials file: {}'.format(str(e)))
            self.credentials = {}
        self.clouds = self.read_clouds()

    def read_clouds(self):
        """Read and return clouds.yaml as a Python dict."""
        try:
            with open(os.path.join(self.juju_home, 'clouds.yaml')) as f:
                return yaml.safe_load(f)
        except IOError as e:
            if e.errno != errno.ENOENT:
                raise RuntimeError(
                    'Failed to read clouds file: {}'.format(str(e)))
            # Default to an empty clouds file.
            return {'clouds': {}}

    @classmethod
    def from_config(cls, name):
        """Create a model from the three configuration files."""
        juju_data = cls._from_config(name)
        juju_data.load_yaml()
        return juju_data

    @classmethod
    def from_cloud_region(cls, cloud, region, config, clouds, juju_home):
        """Return a JujuData for the specified cloud and region.

        :param cloud: The name of the cloud to use.
        :param region: The name of the region to use.  If None, an arbitrary
            region will be selected.
        :param config: The bootstrap config to use.
        :param juju_home: The JUJU_DATA directory to use (credentials are
            loaded from this.)
        """
        cloud_config = clouds['clouds'][cloud]
        provider = cloud_config['type']
        config['type'] = provider
        if provider == 'maas':
            config['maas-server'] = cloud_config['endpoint']
        elif provider == 'openstack':
            config['auth-url'] = cloud_config['endpoint']
        elif provider == 'vsphere':
            config['host'] = cloud_config['endpoint']
        data = JujuData(cloud, config, juju_home, cloud_name=cloud)
        data.load_yaml()
        data.clouds = clouds
        if region is None:
            regions = cloud_config.get('regions', {}).keys()
            if len(regions) > 0:
                region = regions[0]
        data.set_region(region)
        return data

    @classmethod
    def for_existing(cls, juju_data_dir, controller_name, model_name):
        with open(get_bootstrap_config_path(juju_data_dir)) as f:
            all_bootstrap = yaml.load(f)
        ctrl_config = all_bootstrap['controllers'][controller_name]
        config = ctrl_config['controller-config']
        # config is expected to have a 1.x style of config, so mash up
        # controller and model config.
        config.update(ctrl_config['model-config'])
        config['type'] = ctrl_config['type']
        data = cls(
            model_name, config, juju_data_dir, Controller(controller_name),
            ctrl_config['cloud']
            )
        data.set_region(ctrl_config['region'])
        data.load_yaml()
        return data

    def dump_yaml(self, path, config):
        """Dump the configuration files to the specified path.

        config is unused, but is accepted for compatibility with
        SimpleEnvironment and make_jes_home().
        """
        with open(os.path.join(path, 'credentials.yaml'), 'w') as f:
            yaml.safe_dump(self.credentials, f)
        self.write_clouds(path, self.clouds)

    @staticmethod
    def write_clouds(path, clouds):
        with open(os.path.join(path, 'clouds.yaml'), 'w') as f:
            yaml.safe_dump(clouds, f)

    def find_endpoint_cloud(self, cloud_type, endpoint):
        for cloud, cloud_config in self.clouds['clouds'].items():
            if cloud_config['type'] != cloud_type:
                continue
            if cloud_config['endpoint'] == endpoint:
                return cloud
        raise LookupError('No such endpoint: {}'.format(endpoint))

    def get_cloud(self):
        if self._cloud_name is not None:
            return self._cloud_name
        provider = self.provider
        # Separate cloud recommended by: Juju Cloud / Credentials / BootStrap /
        # Model CLI specification
        if provider == 'ec2' and self._config['region'] == 'cn-north-1':
            return 'aws-china'
        if provider not in ('maas', 'openstack', 'vsphere'):
            return {
                'ec2': 'aws',
                'gce': 'google',
            }.get(provider, provider)
        if provider == 'maas':
            endpoint = self._config['maas-server']
        elif provider == 'openstack':
            endpoint = self._config['auth-url']
        elif provider == 'vsphere':
            endpoint = self._config['host']
        return self.find_endpoint_cloud(provider, endpoint)

    def get_cloud_credentials_item(self):
        cloud_name = self.get_cloud()
        cloud = self.credentials['credentials'][cloud_name]
        (credentials_item,) = cloud.items()
        return credentials_item

    def get_cloud_credentials(self):
        """Return the credentials for this model's cloud."""
        return self.get_cloud_credentials_item()[1]

    def is_cloud_provider(self):
        """Return True if the commandline cloud is a provider.

        Examples: lxd, manual
        """
        # if the commandline cloud is "lxd" or "manual", the provider type
        # should match, and shortcutting get_cloud avoids pointless test
        # breakage.
        return bool(self.provider in ('lxd', 'manual') and
                    self.get_cloud() in ('lxd', 'manual'))


class StatusError(Exception):
    """Generic error for Status."""

    recoverable = True

    # This has to be filled in after the classes are declared.
    ordering = []

    @classmethod
    def priority(cls):
        """Get the priority of the StatusError as an number.

        Lower number means higher priority. This can be used as a key
        function in sorting."""
        return cls.ordering.index(cls)


class MachineError(StatusError):
    """Error in machine-status."""

    recoverable = False


class ProvisioningError(MachineError):
    """Machine experianced a 'provisioning error'."""


class StuckAllocatingError(MachineError):
    """Machine did not transition out of 'allocating' state."""

    recoverable = True


class UnitError(StatusError):
    """Error in a unit's status."""


class HookFailedError(UnitError):
    """A unit hook has failed."""

    def __init__(self, item_name, msg):
        match = re.search('^hook failed: "([^"]+)"$', msg)
        if match:
            msg = match.group(1)
        super(HookFailedError, self).__init__(item_name, msg)


class InstallError(HookFailedError):
    """The unit's install hook has failed."""

    recoverable = True


class AppError(StatusError):
    """Error in an application's status."""


class AgentError(StatusError):
    """Error in a juju agent."""


class AgentUnresolvedError(AgentError):
    """Agent error has not recovered in a reasonable time."""

    # This is the time limit set by IS for recovery from an agent error.
    a_reasonable_time = timedelta(minutes=5)


StatusError.ordering = [
    ProvisioningError, StuckAllocatingError, MachineError, InstallError,
    AgentUnresolvedError, HookFailedError, UnitError, AppError, AgentError,
    StatusError,
    ]


class StatusItem:

    APPLICATION = 'application-status'
    WORKLOAD = 'workload-status'
    MACHINE = 'machine-status'
    JUJU = 'juju-status'

    def __init__(self, status_name, item_name, item_value):
        """Create a new StatusItem from its fields.

        :param status_name: One of the status strings.
        :param item_name: The name of the machine/unit/application the status
            information is about.
        :param item_value: A dictionary of status values. If there is an entry
            with the status_name in the dictionary its contents are used."""
        self.status_name = status_name
        self.item_name = item_name
        self.status = item_value.get(status_name, item_value)

    def __eq__(self, other):
        if type(other) != type(self):
            return False
        elif self.status_name != other.status_name:
            return False
        elif self.item_name != other.item_name:
            return False
        elif self.status != other.status:
            return False
        else:
            return True

    def __ne__(self, other):
        return bool(not self == other)

    @property
    def message(self):
        return self.status.get('message')

    @property
    def since(self):
        return self.status.get('since')

    @property
    def current(self):
        return self.status.get('current')

    @property
    def version(self):
        return self.status.get('version')

    @property
    def datetime_since(self):
        if self.since is None:
            return None
        return datetime_parse(self.since)

    def to_exception(self):
        """Create an exception representing the error if one exists.

        :return: StatusError (or subtype) to represent an error or None
        to show that there is no error."""
        if self.current not in ['error', 'failed', 'down',
                                'provisioning error']:
            if (self.current, self.status_name) != (
                    'allocating', self.MACHINE):
                return None
        if self.APPLICATION == self.status_name:
            return AppError(self.item_name, self.message)
        elif self.WORKLOAD == self.status_name:
            if self.message is None:
                return UnitError(self.item_name, self.message)
            elif re.match('hook failed: ".*install.*"', self.message):
                return InstallError(self.item_name, self.message)
            elif re.match('hook failed', self.message):
                return HookFailedError(self.item_name, self.message)
            else:
                return UnitError(self.item_name, self.message)
        elif self.MACHINE == self.status_name:
            if self.current == 'provisioning error':
                return ProvisioningError(self.item_name, self.message)
            if self.current == 'allocating':
                return StuckAllocatingError(
                    self.item_name,
                    'Stuck allocating.  Last message: {}'.format(self.message))
            else:
                return MachineError(self.item_name, self.message)
        elif self.JUJU == self.status_name:
            if self.since is None:
                return AgentError(self.item_name, self.message)
            time_since = datetime.now(tz.gettz('UTC')) - self.datetime_since
            if time_since > AgentUnresolvedError.a_reasonable_time:
                return AgentUnresolvedError(self.item_name, self.message,
                                            time_since.total_seconds())
            else:
                return AgentError(self.item_name, self.message)
        else:
            raise ValueError('Unknown status:{}'.format(self.status_name),
                             (self.item_name, self.status_value))

    def __repr__(self):
        return 'StatusItem({!r}, {!r}, {!r})'.format(
            self.status_name, self.item_name, self.status)


class Status:

    def __init__(self, status, status_text):
        self.status = status
        self.status_text = status_text

    @classmethod
    def from_text(cls, text):
        try:
            # Parsing as JSON is much faster than parsing as YAML, so try
            # parsing as JSON first and fall back to YAML.
            status_yaml = json.loads(text)
        except ValueError:
            status_yaml = yaml.safe_load(text)
        return cls(status_yaml, text)

    @property
    def model_name(self):
        return self.status['model']['name']

    def get_applications(self):
        return self.status.get('applications', {})

    def iter_machines(self, containers=False, machines=True):
        for machine_name, machine in sorted(self.status['machines'].items()):
            if machines:
                yield machine_name, machine
            if containers:
                for contained, unit in machine.get('containers', {}).items():
                    yield contained, unit

    def iter_new_machines(self, old_status, containers=False):
        old = dict(old_status.iter_machines(containers=containers))
        for machine, data in self.iter_machines(containers=containers):
            if machine in old:
                continue
            yield machine, data

    def _iter_units_in_application(self, app_data):
        """Given application data, iterate through every unit in it."""
        for unit_name, unit in sorted(app_data.get('units', {}).items()):
            yield unit_name, unit
            subordinates = unit.get('subordinates', ())
            for sub_name in sorted(subordinates):
                yield sub_name, subordinates[sub_name]

    def iter_units(self):
        """Iterate over every unit in every application."""
        for service_name, service in sorted(self.get_applications().items()):
            for name, data in self._iter_units_in_application(service):
                yield name, data

    def agent_items(self):
        for machine_name, machine in self.iter_machines(containers=True):
            yield machine_name, machine
        for unit_name, unit in self.iter_units():
            yield unit_name, unit

    def unit_agent_states(self, states=None):
        """Fill in a dictionary with the states of units.

        Units of a dying application are marked as dying.

        :param states: If not None, when it should be a defaultdict(list)),
        then states are added to this dictionary."""
        if states is None:
            states = defaultdict(list)
        for app_name, app_data in sorted(self.get_applications().items()):
            if app_data.get('life') == 'dying':
                for unit, data in self._iter_units_in_application(app_data):
                    states['dying'].append(unit)
            else:
                for unit, data in self._iter_units_in_application(app_data):
                    states[coalesce_agent_status(data)].append(unit)
        return states

    def agent_states(self):
        """Map agent states to the units and machines in those states."""
        states = defaultdict(list)
        for item_name, item in self.iter_machines(containers=True):
            states[coalesce_agent_status(item)].append(item_name)
        self.unit_agent_states(states)
        return states

    def check_agents_started(self, environment_name=None):
        """Check whether all agents are in the 'started' state.

        If not, return agent_states output.  If so, return None.
        If an error is encountered for an agent, raise ErroredUnit
        """
        bad_state_info = re.compile(
            '(.*error|^(cannot set up groups|cannot run instance)).*')
        for item_name, item in self.agent_items():
            state_info = item.get('agent-state-info', '')
            if bad_state_info.match(state_info):
                raise ErroredUnit(item_name, state_info)
        states = self.agent_states()
        if set(states.keys()).issubset(AGENTS_READY):
            return None
        for state, entries in states.items():
            if 'error' in state:
                # sometimes the state may be hidden in juju status message
                juju_status = dict(
                    self.agent_items())[entries[0]].get('juju-status')
                if juju_status:
                    juju_status_msg = juju_status.get('message')
                    if juju_status_msg:
                        state = juju_status_msg
                raise ErroredUnit(entries[0], state)
        return states

    def get_service_count(self):
        return len(self.get_applications())

    def get_service_unit_count(self, service):
        return len(
            self.get_applications().get(service, {}).get('units', {}))

    def get_agent_versions(self):
        versions = defaultdict(set)
        for item_name, item in self.agent_items():
            if item.get('juju-status', None):
                version = item['juju-status'].get('version', 'unknown')
                versions[version].add(item_name)
            else:
                versions[item.get('agent-version', 'unknown')].add(item_name)
        return versions

    def get_instance_id(self, machine_id):
        return self.status['machines'][machine_id]['instance-id']

    def get_machine_dns_name(self, machine_id):
        return _dns_name_for_machine(self, machine_id)

    def get_unit(self, unit_name):
        """Return metadata about a unit."""
        for name, service in sorted(self.get_applications().items()):
            if unit_name in service.get('units', {}):
                return service['units'][unit_name]
        raise KeyError(unit_name)

    def service_subordinate_units(self, service_name):
        """Return subordinate metadata for a service_name."""
        services = self.get_applications()
        if service_name in services:
            for name, unit in sorted(services[service_name].get(
                    'units', {}).items()):
                for sub_name, sub in unit.get('subordinates', {}).items():
                    yield sub_name, sub

    def get_open_ports(self, unit_name):
        """List the open ports for the specified unit.

        If no ports are listed for the unit, the empty list is returned.
        """
        return self.get_unit(unit_name).get('open-ports', [])

    def iter_status(self):
        """Iterate through every status field in the larger status data."""
        for machine_name, machine_value in self.iter_machines(containers=True):
            yield StatusItem(StatusItem.MACHINE, machine_name, machine_value)
            yield StatusItem(StatusItem.JUJU, machine_name, machine_value)
        for app_name, app_value in self.get_applications().items():
            yield StatusItem(StatusItem.APPLICATION, app_name, app_value)
            unit_iterator = self._iter_units_in_application(app_value)
            for unit_name, unit_value in unit_iterator:
                yield StatusItem(StatusItem.WORKLOAD, unit_name, unit_value)
                yield StatusItem(StatusItem.JUJU, unit_name, unit_value)

    def iter_errors(self, ignore_recoverable=False):
        """Iterate through every error, repersented by exceptions."""
        for sub_status in self.iter_status():
            error = sub_status.to_exception()
            if error is not None:
                if not (ignore_recoverable and error.recoverable):
                    yield error

    def check_for_errors(self, ignore_recoverable=False):
        """Return a list of errors, in order of their priority."""
        return sorted(self.iter_errors(ignore_recoverable),
                      key=lambda item: item.priority())

    def raise_highest_error(self, ignore_recoverable=False):
        """Raise an exception reperenting the highest priority error."""
        errors = self.check_for_errors(ignore_recoverable)
        if errors:
            raise errors[0]


def describe_substrate(env):
    if env.provider == 'local':
        return {
            'kvm': 'KVM (local)',
            'lxc': 'LXC (local)'
        }[env.get_option('container', 'lxc')]
    elif env.provider == 'openstack':
        if env.get_option('auth-url') == (
                'https://keystone.canonistack.canonical.com:443/v2.0/'):
            return 'Canonistack'
        else:
            return 'Openstack'
    try:
        return {
            'ec2': 'AWS',
            'rackspace': 'Rackspace',
            'joyent': 'Joyent',
            'azure': 'Azure',
            'maas': 'MAAS',
        }[env.provider]
    except KeyError:
        return env.provider


class Juju2Backend:
    """A Juju backend referring to a specific juju 2 binary.

    Uses -m to specify models, uses JUJU_DATA to specify home directory.
    """

    _model_flag = '-m'

    def __init__(self, full_path, version, feature_flags, debug,
                 soft_deadline=None):
        self._version = version
        self._full_path = full_path
        self.feature_flags = feature_flags
        self.debug = debug
        self._timeout_path = get_timeout_path()
        self.juju_timings = []
        self.soft_deadline = soft_deadline
        self._ignore_soft_deadline = False

    def _now(self):
        return datetime.utcnow()

    @contextmanager
    def _check_timeouts(self):
        # If an exception occurred, we don't want to replace it with
        # SoftDeadlineExceeded.
        yield
        if self.soft_deadline is None or self._ignore_soft_deadline:
            return
        if self._now() > self.soft_deadline:
            raise SoftDeadlineExceeded()

    @contextmanager
    def ignore_soft_deadline(self):
        """Ignore the client deadline.  For cleanup code."""
        old_val = self._ignore_soft_deadline
        self._ignore_soft_deadline = True
        try:
            yield
        finally:
            self._ignore_soft_deadline = old_val

    def clone(self, full_path, version, debug, feature_flags):
        if version is None:
            version = self.version
        if full_path is None:
            full_path = self.full_path
        if debug is None:
            debug = self.debug
        result = self.__class__(full_path, version, feature_flags, debug,
                                self.soft_deadline)
        # Each clone shares a reference to juju_timings allowing us to collect
        # all commands run during a test.
        result.juju_timings = self.juju_timings
        return result

    @property
    def version(self):
        return self._version

    @property
    def full_path(self):
        return self._full_path

    @property
    def juju_name(self):
        return os.path.basename(self._full_path)

    def _get_attr_tuple(self):
        return (self._version, self._full_path, self.feature_flags,
                self.debug, self.juju_timings)

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        return self._get_attr_tuple() == other._get_attr_tuple()

    def shell_environ(self, used_feature_flags, juju_home):
        """Generate a suitable shell environment.

        Juju's directory must be in the PATH to support plugins.
        """
        env = dict(os.environ)
        if self.full_path is not None:
            env['PATH'] = '{}{}{}'.format(os.path.dirname(self.full_path),
                                          os.pathsep, env['PATH'])
        flags = self.feature_flags.intersection(used_feature_flags)
        feature_flag_string = env.get(JUJU_DEV_FEATURE_FLAGS, '')
        if feature_flag_string != '':
            flags.update(feature_flag_string.split(','))
        if flags:
            env[JUJU_DEV_FEATURE_FLAGS] = ','.join(sorted(flags))
        env['JUJU_DATA'] = juju_home
        return env

    def full_args(self, command, args, model, timeout):
        if model is not None:
            e_arg = (self._model_flag, model)
        else:
            e_arg = ()
        if timeout is None:
            prefix = ()
        else:
            prefix = get_timeout_prefix(timeout, self._timeout_path)
        logging = '--debug' if self.debug else '--show-log'

        # If args is a string, make it a tuple. This makes writing commands
        # with one argument a bit nicer.
        if isinstance(args, argtype):
            args = (args,)
        # we split the command here so that the caller can control where the -m
        # model flag goes.  Everything in the command string is put before the
        # -m flag.
        command = command.split()
        return (prefix + (self.juju_name, logging,) + tuple(command) + e_arg +
                args)

    def juju(self, command, args, used_feature_flags,
             juju_home, model=None, check=True, timeout=None, extra_env=None,
             suppress_err=False):
        """Run a command under juju for the current environment.

        :return: Tuple rval, CommandTime rval being the commands exit code and
          a CommandTime object used for storing command timing data.
        """
        args = self.full_args(command, args, model, timeout)
        log.info(' '.join(args))
        env = self.shell_environ(used_feature_flags, juju_home)
        if extra_env is not None:
            env.update(extra_env)
        if check:
            call_func = subprocess.check_call
        else:
            call_func = subprocess.call
        # Mutate os.environ instead of supplying env parameter so Windows can
        # search env['PATH']
        stderr = subprocess.PIPE if suppress_err else None
        # Keep track of commands and how long the take.
        command_time = CommandTime(command, args, env)
        with scoped_environ(env):
            log.debug('Running juju with env: {}'.format(env))
            with self._check_timeouts():
                rval = call_func(args, stderr=stderr)
        self.juju_timings.append(command_time)
        return rval, command_time

    def expect(self, command, args, used_feature_flags, juju_home, model=None,
               timeout=None, extra_env=None):
        args = self.full_args(command, args, model, timeout)
        log.info(' '.join(args))
        env = self.shell_environ(used_feature_flags, juju_home)
        if extra_env is not None:
            env.update(extra_env)
        # pexpect.spawn expects a string. This is better than trying to extract
        # command + args from the returned tuple (as there could be an intial
        # timing command tacked on).
        command_string = ' '.join(quote(a) for a in args)
        with scoped_environ(env):
            return pexpect.spawn(command_string)

    @contextmanager
    def juju_async(self, command, args, used_feature_flags,
                   juju_home, model=None, timeout=None):
        full_args = self.full_args(command, args, model, timeout)
        log.info(' '.join(args))
        env = self.shell_environ(used_feature_flags, juju_home)
        # Mutate os.environ instead of supplying env parameter so Windows can
        # search env['PATH']
        with scoped_environ(env):
            with self._check_timeouts():
                proc = subprocess.Popen(full_args)
        yield proc
        retcode = proc.wait()
        if retcode != 0:
            raise subprocess.CalledProcessError(retcode, full_args)

    def get_juju_output(self, command, args, used_feature_flags, juju_home,
                        model=None, timeout=None, user_name=None,
                        merge_stderr=False):
        args = self.full_args(command, args, model, timeout)
        env = self.shell_environ(used_feature_flags, juju_home)
        log.debug(args)
        # Mutate os.environ instead of supplying env parameter so
        # Windows can search env['PATH']
        with scoped_environ(env):
            proc = subprocess.Popen(
                args, stdout=subprocess.PIPE, stdin=subprocess.PIPE,
                stderr=subprocess.STDOUT if merge_stderr else subprocess.PIPE)
            with self._check_timeouts():
                sub_output, sub_error = proc.communicate()
            log.debug(sub_output)
            if proc.returncode != 0:
                log.debug(sub_error)
                e = subprocess.CalledProcessError(
                    proc.returncode, args, sub_output)
                e.stderr = sub_error
                if sub_error and (
                    b'Unable to connect to environment' in sub_error or
                        b'MissingOrIncorrectVersionHeader' in sub_error or
                        b'307: Temporary Redirect' in sub_error):
                    raise CannotConnectEnv(e)
                raise e
        return sub_output

    def get_active_model(self, juju_data_dir):
        """Determine the active model in a juju data dir."""
        try:
            current = json.loads(self.get_juju_output(
                'models', ('--format', 'json'), set(),
                juju_data_dir, model=None).decode('ascii'))
        except subprocess.CalledProcessError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))
        try:
            return current['current-model']
        except KeyError:
            raise NoActiveModel('No active model for {}'.format(juju_data_dir))

    def get_active_controller(self, juju_data_dir):
        """Determine the active controller in a juju data dir."""
        try:
            current = json.loads(self.get_juju_output(
                'controllers', ('--format', 'json'), set(),
                juju_data_dir, model=None).decode('ascii'))
        except subprocess.CalledProcessError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))
        try:
            return current['current-controller']
        except KeyError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))

    def get_active_user(self, juju_data_dir, controller):
        """Determine the active user for a controller."""
        try:
            current = json.loads(self.get_juju_output(
                'controllers', ('--format', 'json'), set(),
                juju_data_dir, model=None).decode('ascii'))
        except subprocess.CalledProcessError:
            raise NoActiveControllers(
                'No active controller for {}'.format(juju_data_dir))
        return current['controllers'][controller]['user']

    def pause(self, seconds):
        pause(seconds)


class BaseCondition:
    """Base class for conditions that support client.wait_for."""

    def __init__(self, timeout=300, already_satisfied=False):
        self.timeout = timeout
        self.already_satisfied = already_satisfied

    def iter_blocking_state(self, status):
        """Identify when the condition required is met.

        When the operation is complete yield nothing. Otherwise yields a
        tuple ('<item detail>', '<state>')
        as to why the action cannot be considered complete yet.

        An example for a condition of an application being removed:
            yield <application name>, 'still-present'
        """
        raise NotImplementedError()

    def do_raise(self, model_name, status):
        """Raise exception for when success condition fails to be achieved."""
        raise NotImplementedError()


class ConditionList(BaseCondition):
    """A list of conditions that support client.wait_for.

    This combines the supplied list of conditions.  It is only satisfied when
    all conditions are met.  It times out when any member times out.  When
    asked to raise, it causes the first condition to raise an exception.  An
    improvement would be to raise the first condition whose timeout has been
    exceeded.
    """

    def __init__(self, conditions):
        if len(conditions) == 0:
            timeout = 300
        else:
            timeout = max(c.timeout for c in conditions)
        already_satisfied = all(c.already_satisfied for c in conditions)
        super(ConditionList, self).__init__(timeout, already_satisfied)
        self._conditions = conditions

    def iter_blocking_state(self, status):
        for condition in self._conditions:
            for item, state in condition.iter_blocking_state(status):
                yield item, state

    def do_raise(self, model_name, status):
        self._conditions[0].do_raise(model_name, status)


class NoopCondition(BaseCondition):

    def iter_blocking_state(self, status):
        return iter(())

    def do_raise(self, model_name, status):
        raise Exception('NoopCondition failed: {}'.format(model_name))


class WaitMachineNotPresent(BaseCondition):
    """Condition satisfied when a given machine is not present."""

    def __init__(self, machine, timeout=300):
        super(WaitMachineNotPresent, self).__init__(timeout)
        self.machine = machine

    def __eq__(self, other):
        if not type(self) is type(other):
            return False
        if self.timeout != other.timeout:
            return False
        if self.machine != other.machine:
            return False
        return True

    def __ne__(self, other):
        return not self.__eq__(other)

    def iter_blocking_state(self, status):
        for machine, info in status.iter_machines():
            if machine == self.machine:
                yield machine, 'still-present'

    def do_raise(self, model_name, status):
        raise Exception("Timed out waiting for machine removal %s" %
                        self.machine)


class WaitApplicationNotPresent(BaseCondition):
    """Condition satisfied when a given machine is not present."""

    def __init__(self, application, timeout=300):
        super(WaitApplicationNotPresent, self).__init__(timeout)
        self.application = application

    def __eq__(self, other):
        if not type(self) is type(other):
            return False
        if self.timeout != other.timeout:
            return False
        if self.application != other.application:
            return False
        return True

    def __ne__(self, other):
        return not self.__eq__(other)

    def iter_blocking_state(self, status):
        for application in status.get_applications().keys():
            if application == self.application:
                yield application, 'still-present'

    def do_raise(self, model_name, status):
        raise Exception("Timed out waiting for application "
                        "removal {}".format(self.application))


class MachineDown(BaseCondition):
    """Condition satisfied when a given machine is down."""

    def __init__(self, machine_id):
        super(MachineDown, self).__init__()
        self.machine_id = machine_id

    def iter_blocking_state(self, status):
        """Yield the juju-status of the machine if it is not 'down'."""
        juju_status = status.status['machines'][self.machine_id]['juju-status']
        if juju_status['current'] != 'down':
            yield self.machine_id, juju_status['current']

    def do_raise(self, model_name, status):
        raise Exception(
            "Timed out waiting for juju to determine machine {} down.".format(
                self.machine_id))


class WaitVersion(BaseCondition):

    def __init__(self, target_version, timeout=300):
        super(WaitVersion, self).__init__(timeout)
        self.target_version = target_version

    def iter_blocking_state(self, status):
        for version, agents in status.get_agent_versions().items():
            if version == self.target_version:
                continue
            for agent in agents:
                yield agent, version

    def do_raise(self, model_name, status):
        raise VersionsNotUpdated(model_name, status)


class WaitAgentsStarted(BaseCondition):
    """Wait until all agents are idle or started."""

    def __init__(self, timeout=1200):
        super(WaitAgentsStarted, self).__init__(timeout)

    def iter_blocking_state(self, status):
        states = Status.check_agents_started(status)

        if states is not None:
            for state, item in states.items():
                yield item[0], state

    def do_raise(self, model_name, status):
        raise AgentsNotStarted(model_name, status)


class CommandTime:
    """Store timing details for a juju command."""

    def __init__(self, cmd, full_args, envvars=None, start=None):
        """Constructor.

        :param cmd: Command string for command run (e.g. bootstrap)
        :param args: List of all args the command was called with.
        :param envvars: Dict of any extra envvars set before command was
          called.
        :param start: datetime.datetime object representing when the command
          was run. If None defaults to datetime.utcnow()
        """
        self.cmd = cmd
        self.full_args = full_args
        self.envvars = envvars
        self.start = start if start else datetime.utcnow()
        self.end = None

    def actual_completion(self, end=None):
        """Signify that actual completion time of the command.

        Note. ignores multiple calls after the initial call.

        :param end: datetime.datetime object. If None defaults to
          datetime.datetime.utcnow()
        """
        if self.end is None:
            self.end = end if end else datetime.utcnow()

    @property
    def total_seconds(self):
        """Total amount of seconds a command took to complete.

        :return: Int representing number of seconds or None if the command
          timing has never been completed.
        """
        if self.end is None:
            return None
        return (self.end - self.start).total_seconds()


class CommandComplete(BaseCondition):
    """Wraps a CommandTime and gives the ability to wait_for completion."""

    def __init__(self, real_condition, command_time):
        """Constructor.

        :param real_condition: BaseCondition object.
        :param command_time: CommandTime object representing the command to
          wait for completion.
        """
        super(CommandComplete, self).__init__(
            real_condition.timeout,
            real_condition.already_satisfied)
        self._real_condition = real_condition
        self.command_time = command_time
        if real_condition.already_satisfied:
            self.command_time.actual_completion()

    def iter_blocking_state(self, status):
        """Wraps the iter_blocking_state of the stored BaseCondition.

        When the operation is complete iter_blocking_state yields nothing.
        Otherwise iter_blocking_state yields details as to why the action
        cannot be considered complete yet.
        """
        completed = True
        for item, state in self._real_condition.iter_blocking_state(status):
            completed = False
            yield item, state
        if completed:
            self.command_time.actual_completion()

    def do_raise(self, status):
        raise RuntimeError(
            'Timed out waiting for "{}" command to complete: "{}"'.format(
                self.command_time.cmd,
                ' '.join(self.command_time.full_args)))


def get_stripped_version_number(version_string):
    # strip the series and arch from the built version.
    version_parts = version_string.split('-')
    if len(version_parts) == 4:
        version_number = '-'.join(version_parts[0:2])
    else:
        version_number = version_parts[0]
    return version_number


class ModelClient:
    """Wraps calls to a juju instance, associated with a single model.

    Note: A model is often called an enviroment (Juju 1 legacy).

    This class represents the latest Juju version.
    """

    # The environments.yaml options that are replaced by bootstrap options.
    #
    # As described in bug #1538735, default-series and --bootstrap-series must
    # match.  'default-series' should be here, but is omitted so that
    # default-series is always forced to match --bootstrap-series.
    bootstrap_replaces = frozenset(['agent-version'])

    # What feature flags have existed that CI used.
    known_feature_flags = frozenset(['actions', 'jes', 'migration'])

    # What feature flags are used by this version of the juju client.
    used_feature_flags = frozenset(['migration'])

    destroy_model_command = 'destroy-model'

    supported_container_types = frozenset([KVM_MACHINE, LXC_MACHINE,
                                           LXD_MACHINE])

    default_backend = Juju2Backend

    config_class = JujuData

    status_class = Status

    agent_metadata_url = 'agent-metadata-url'

    model_permissions = frozenset(['read', 'write', 'admin'])

    controller_permissions = frozenset(['login', 'addmodel', 'superuser'])

    reserved_spaces = frozenset([
        'endpoint-bindings-data', 'endpoint-bindings-public'])

    command_set_destroy_model = 'destroy-model'

    command_set_remove_object = 'remove-object'

    command_set_all = 'all'

    REGION_ENDPOINT_PROMPT = (
        r'Enter the API endpoint url for the region \[use cloud api url\]:')

    login_user_command = 'login -u'

    @classmethod
    def preferred_container(cls):
        for container_type in [LXD_MACHINE, LXC_MACHINE]:
            if container_type in cls.supported_container_types:
                return container_type

    _show_status = 'show-status'

    @classmethod
    def get_version(cls, juju_path=None):
        """Get the version data from a juju binary.

        :param juju_path: Path to binary. If not given or None, 'juju' is used.
        """
        if juju_path is None:
            juju_path = 'juju'
        version = subprocess.check_output((juju_path, '--version')).strip()
        return version.decode("utf-8")

    def check_timeouts(self):
        return self._backend._check_timeouts()

    def ignore_soft_deadline(self):
        return self._backend.ignore_soft_deadline()

    def enable_feature(self, flag):
        """Enable juju feature by setting the given flag.

        New versions of juju with the feature enabled by default will silently
        allow this call, but will not export the environment variable.
        """
        if flag not in self.known_feature_flags:
            raise ValueError('Unknown feature flag: %r' % (flag,))
        self.feature_flags.add(flag)

    def get_jes_command(self):
        """For Juju 2.0, this is always kill-controller."""
        return KILL_CONTROLLER

    def is_jes_enabled(self):
        """Does the state-server support multiple environments."""
        try:
            self.get_jes_command()
            return True
        except JESNotSupported:
            return False

    def enable_jes(self):
        """Enable JES if JES is optional.

        Specifically implemented by the clients that optionally support JES.
        This version raises either JESByDefault or JESNotSupported.

        :raises: JESByDefault when JES is always enabled; Juju has the
            'destroy-controller' command.
        :raises: JESNotSupported when JES is not supported; Juju does not have
            the 'system kill' command when the JES feature flag is set.
        """
        if self.is_jes_enabled():
            raise JESByDefault()
        else:
            raise JESNotSupported()

    @classmethod
    def get_full_path(cls):
        if sys.platform == 'win32':
            return WIN_JUJU_CMD
        return subprocess.check_output(
            ('which', 'juju')).decode(getpreferredencoding()).rstrip('\n')

    def clone_from_path(self, juju_path):
        """Clone using the supplied path."""
        if juju_path is None:
            full_path = self.get_full_path()
        else:
            full_path = os.path.abspath(juju_path)
        return self.clone(
            full_path=full_path, version=self.get_version(juju_path))

    def clone(self, env=None, version=None, full_path=None, debug=None,
              cls=None):
        """Create a clone of this ModelClient.

        By default, the class, environment, version, full_path, and debug
        settings will match the original, but each can be overridden.
        """
        if env is None:
            env = self.env
        if cls is None:
            cls = self.__class__
        feature_flags = self.feature_flags.intersection(cls.used_feature_flags)
        backend = self._backend.clone(full_path, version, debug, feature_flags)
        other = cls.from_backend(backend, env)
        other.excluded_spaces = set(self.excluded_spaces)
        return other

    @classmethod
    def from_backend(cls, backend, env):
        return cls(env=env, version=backend.version,
                   full_path=backend.full_path,
                   debug=backend.debug, _backend=backend)

    def get_cache_path(self):
        return get_cache_path(self.env.juju_home, models=True)

    def _cmd_model(self, include_e, controller):
        if controller:
            return '{controller}:{model}'.format(
                controller=self.env.controller.name,
                model=self.get_controller_model_name())
        elif self.env is None or not include_e:
            return None
        else:
            return '{controller}:{model}'.format(
                controller=self.env.controller.name,
                model=self.model_name)

    @staticmethod
    def _get_env(env):
        if not isinstance(env, JujuData) and isinstance(env,
                                                        SimpleEnvironment):
            # FIXME: JujuData should be used from the start.
            env = JujuData.from_env(env)
        return env

    def __init__(self, env, version, full_path, juju_home=None, debug=False,
                 soft_deadline=None, _backend=None):
        """Create a new juju client.

        Required Arguments
        :param env: Object representing a model in a data directory.
        :param version: Version of juju the client wraps.
        :param full_path: Full path to juju binary.

        Optional Arguments
        :param juju_home: default value for env.juju_home.  Will be
            autodetected if None (the default).
        :param debug: Flag to activate debugging output; False by default.
        :param soft_deadline: A datetime representing the deadline by which
            normal operations should complete.  If None, no deadline is
            enforced.
        :param _backend: The backend to use for interacting with the client.
            If None (the default), self.default_backend will be used.
        """
        self.env = self._get_env(env)
        if _backend is None:
            _backend = self.default_backend(full_path, version, set(), debug,
                                            soft_deadline)
        self._backend = _backend
        if version != _backend.version:
            raise ValueError('Version mismatch: {} {}'.format(
                version, _backend.version))
        if full_path != _backend.full_path:
            raise ValueError('Path mismatch: {} {}'.format(
                full_path, _backend.full_path))
        if debug is not _backend.debug:
            raise ValueError('debug mismatch: {} {}'.format(
                debug, _backend.debug))
        if env is not None:
            if juju_home is None:
                if env.juju_home is None:
                    env.juju_home = get_juju_home()
            else:
                env.juju_home = juju_home
        self.excluded_spaces = set(self.reserved_spaces)

    @property
    def version(self):
        return self._backend.version

    @property
    def full_path(self):
        return self._backend.full_path

    @property
    def feature_flags(self):
        return self._backend.feature_flags

    @feature_flags.setter
    def feature_flags(self, feature_flags):
        self._backend.feature_flags = feature_flags

    @property
    def debug(self):
        return self._backend.debug

    @property
    def model_name(self):
        return self.env.environment

    def _shell_environ(self):
        """Generate a suitable shell environment.

        Juju's directory must be in the PATH to support plugins.
        """
        return self._backend.shell_environ(self.used_feature_flags,
                                           self.env.juju_home)

    def use_reserved_spaces(self, spaces):
        """Allow machines in given spaces to be allocated and used."""
        if not self.reserved_spaces.issuperset(spaces):
            raise ValueError('Space not reserved: {}'.format(spaces))
        self.excluded_spaces.difference_update(spaces)

    def add_ssh_machines(self, machines):
        for count, machine in enumerate(machines):
            try:
                self.juju('add-machine', ('ssh:' + machine,))
            except subprocess.CalledProcessError:
                if count != 0:
                    raise
                logging.warning('add-machine failed.  Will retry.')
                pause(30)
                self.juju('add-machine', ('ssh:' + machine,))

    def make_remove_machine_condition(self, machine):
        """Return a condition object representing a machine removal.

        The timeout varies depending on the provider.
        See wait_for.
        """
        if self.env.provider == 'azure':
            timeout = 1200
        else:
            timeout = 600
        return WaitMachineNotPresent(machine, timeout)

    def remove_machine(self, machine_id, force=False):
        """Remove a machine (or container).

        :param machine_id: The id of the machine to remove.
        :return: A WaitMachineNotPresent instance for client.wait_for.
        """
        if force:
            options = ('--force',)
        else:
            options = ()
        self.juju('remove-machine', options + (machine_id,))
        return self.make_remove_machine_condition(machine_id)

    @staticmethod
    def get_cloud_region(cloud, region):
        if region is None:
            return cloud
        return '{}/{}'.format(cloud, region)

    def get_bootstrap_args(
            self, upload_tools, config_filename, bootstrap_series=None,
            credential=None, auto_upgrade=False, metadata_source=None,
            no_gui=False, agent_version=None):
        """Return the bootstrap arguments for the substrate."""
        constraints = self._get_substrate_constraints()
        cloud_region = self.get_cloud_region(self.env.get_cloud(),
                                             self.env.get_region())
        # Note cloud_region before controller name
        args = ['--constraints', constraints,
                cloud_region,
                self.env.environment,
                '--config', config_filename,
                '--default-model', self.env.environment]
        if upload_tools:
            if agent_version is not None:
                raise ValueError(
                    'agent-version may not be given with upload-tools.')
            args.insert(0, '--upload-tools')
        else:
            if agent_version is None:
                agent_version = self.get_matching_agent_version()
            args.extend(['--agent-version', agent_version])
        if bootstrap_series is not None:
            args.extend(['--bootstrap-series', bootstrap_series])
        if credential is not None:
            args.extend(['--credential', credential])
        if metadata_source is not None:
            args.extend(['--metadata-source', metadata_source])
        if auto_upgrade:
            args.append('--auto-upgrade')
        if self.env.bootstrap_to is not None:
            args.extend(['--to', self.env.bootstrap_to])
        if no_gui:
            args.append('--no-gui')
        return tuple(args)

    def add_model(self, env):
        """Add a model to this model's controller and return its client.

        :param env: Either a class representing the new model/environment
            or the name of the new model/environment which will then be
            otherwise identical to the current model/environment."""
        if not isinstance(env, SimpleEnvironment):
            env = self.env.clone(env)
        model_client = self.clone(env)
        with model_client._bootstrap_config() as config_file:
            self._add_model(env.environment, config_file)
        return model_client

    def make_model_config(self):
        config_dict = make_safe_config(self)
        agent_metadata_url = config_dict.pop('tools-metadata-url', None)
        if agent_metadata_url is not None:
            config_dict.setdefault('agent-metadata-url', agent_metadata_url)
        # Strip unneeded variables.
        return dict((k, v) for k, v in config_dict.items() if k not in {
            'access-key',
            'api-port',
            'admin-secret',
            'application-id',
            'application-password',
            'auth-url',
            'bootstrap-host',
            'client-email',
            'client-id',
            'control-bucket',
            'host',
            'location',
            'maas-oauth',
            'maas-server',
            'management-certificate',
            'management-subscription-id',
            'manta-key-id',
            'manta-user',
            'max-logs-age',
            'max-logs-size',
            'max-txn-log-size',
            'name',
            'password',
            'private-key',
            'region',
            'sdc-key-id',
            'sdc-url',
            'sdc-user',
            'secret-key',
            'set-numa-control-policy',
            'state-port',
            'storage-account-name',
            'subscription-id',
            'tenant-id',
            'tenant-name',
            'type',
            'username',
        })

    @contextmanager
    def _bootstrap_config(self):
        with temp_yaml_file(self.make_model_config()) as config_filename:
            yield config_filename

    def _check_bootstrap(self):
        if self.env.environment != self.env.controller.name:
            raise AssertionError(
                'Controller and environment names should not vary (yet)')

    def update_user_name(self):
        self.env.user_name = 'admin'

    def bootstrap(self, upload_tools=False, bootstrap_series=None,
                  credential=None, auto_upgrade=False, metadata_source=None,
                  no_gui=False, agent_version=None):
        """Bootstrap a controller."""
        self._check_bootstrap()
        with self._bootstrap_config() as config_filename:
            args = self.get_bootstrap_args(
                upload_tools, config_filename, bootstrap_series, credential,
                auto_upgrade, metadata_source, no_gui, agent_version)
            self.update_user_name()
            retvar, ct = self.juju('bootstrap', args, include_e=False)
            ct.actual_completion()
            return retvar

    @contextmanager
    def bootstrap_async(self, upload_tools=False, bootstrap_series=None,
                        auto_upgrade=False, metadata_source=None,
                        no_gui=False):
        self._check_bootstrap()
        with self._bootstrap_config() as config_filename:
            args = self.get_bootstrap_args(
                upload_tools, config_filename, bootstrap_series, None,
                auto_upgrade, metadata_source, no_gui)
            self.update_user_name()
            with self.juju_async('bootstrap', args, include_e=False):
                yield
                log.info('Waiting for bootstrap of {}.'.format(
                    self.env.environment))

    def _add_model(self, model_name, config_file):
        explicit_region = self.env.controller.explicit_region
        if explicit_region:
            credential_name = self.env.get_cloud_credentials_item()[0]
            cloud_region = self.get_cloud_region(self.env.get_cloud(),
                                                 self.env.get_region())
            region_args = (cloud_region, '--credential', credential_name)
        else:
            region_args = ()
        self.controller_juju('add-model', (model_name,) + region_args +
                             ('--config', config_file,))

    def destroy_model(self):
        exit_status, _ = self.juju(
            'destroy-model', ('{}:{}'.format(self.env.controller.name,
                                             self.env.environment), '-y',),
            include_e=False, timeout=get_teardown_timeout(self))
        return exit_status

    def kill_controller(self, check=False):
        """Kill a controller and its models. Hard kill option.

        :return: Tuple: Subprocess's exit code, CommandComplete object.
        """
        retvar, ct = self.juju(
            'kill-controller', (self.env.controller.name, '-y'),
            include_e=False, check=check, timeout=get_teardown_timeout(self))
        # Already satisfied as this is a sync, operation.
        ct.actual_completion()
        return retvar

    def destroy_controller(self, all_models=False):
        """Destroy a controller and its models. Soft kill option.

        :param all_models: If true will attempt to destroy all the
            controller's models as well.
        :raises: subprocess.CalledProcessError if the operation fails.
        :return: Tuple: Subprocess's exit code, CommandComplete object.
        """
        args = (self.env.controller.name, '-y')
        if all_models:
            args += ('--destroy-all-models',)
        retvar, ct = self.juju(
            'destroy-controller', args, include_e=False,
            timeout=get_teardown_timeout(self))
        # Already satisfied as this is a sync, operation.
        ct.actual_completion()
        return retvar

    def tear_down(self):
        """Tear down the client as cleanly as possible.

        Attempts to use the soft method destroy_controller, if that fails
        it will use the hard kill_controller and raise an error."""
        try:
            self.destroy_controller(all_models=True)
        except subprocess.CalledProcessError:
            logging.warning('tear_down destroy-controller failed')
            retval = self.kill_controller()
            message = 'tear_down kill-controller result={}'.format(retval)
            if retval == 0:
                logging.info(message)
            else:
                logging.warning(message)
            raise

    def get_juju_output(self, command, *args, **kwargs):
        """Call a juju command and return the output.

        Sub process will be called as 'juju <command> <args> <kwargs>'. Note
        that <command> may be a space delimited list of arguments. The -e
        <environment> flag will be placed after <command> and before args.
        """
        model = self._cmd_model(kwargs.get('include_e', True),
                                kwargs.get('controller', False))
        pass_kwargs = dict(
            (k, kwargs[k]) for k in kwargs if k in ['timeout', 'merge_stderr'])
        return self._backend.get_juju_output(
            command, args, self.used_feature_flags, self.env.juju_home,
            model, user_name=self.env.user_name, **pass_kwargs)

    def show_status(self):
        """Print the status to output."""
        self.juju(self._show_status, ('--format', 'yaml'))

    def get_status(self, timeout=60, raw=False, controller=False, *args):
        """Get the current status as a dict."""
        # GZ 2015-12-16: Pass remaining timeout into get_juju_output call.
        for ignored in until_timeout(timeout):
            try:
                if raw:
                    return self.get_juju_output(self._show_status, *args)
                return self.status_class.from_text(
                    self.get_juju_output(
                        self._show_status, '--format', 'yaml',
                        controller=controller).decode('utf-8'))
            except subprocess.CalledProcessError:
                pass
        raise StatusTimeout(
            'Timed out waiting for juju status to succeed')

    def show_model(self, model_name=None):
        model_details = self.get_juju_output(
            'show-model',
            '{}:{}'.format(
                self.env.controller.name, model_name or self.env.environment),
            '--format', 'yaml',
            include_e=False)
        return yaml.safe_load(model_details)

    @staticmethod
    def _dict_as_option_strings(options):
        return tuple('{}={}'.format(*item) for item in options.items())

    def set_config(self, service, options):
        option_strings = self._dict_as_option_strings(options)
        self.juju('config', (service,) + option_strings)

    def get_config(self, service):
        return yaml.safe_load(self.get_juju_output('config', service))

    def get_service_config(self, service, timeout=60):
        for ignored in until_timeout(timeout):
            try:
                return self.get_config(service)
            except subprocess.CalledProcessError:
                pass
        raise Exception(
            'Timed out waiting for juju get %s' % (service))

    def set_model_constraints(self, constraints):
        constraint_strings = self._dict_as_option_strings(constraints)
        retvar, ct = self.juju('set-model-constraints', constraint_strings)
        return retvar, CommandComplete(NoopCondition(), ct)

    def get_model_config(self):
        """Return the value of the environment's configured options."""
        return yaml.safe_load(
            self.get_juju_output('model-config', '--format', 'yaml'))

    def get_env_option(self, option):
        """Return the value of the environment's configured option."""
        return self.get_juju_output(
            'model-config', option).decode(getpreferredencoding())

    def set_env_option(self, option, value):
        """Set the value of the option in the environment."""
        option_value = "%s=%s" % (option, value)
        retvar, ct = self.juju('model-config', (option_value,))
        return CommandComplete(NoopCondition(), ct)

    def unset_env_option(self, option):
        """Unset the value of the option in the environment."""
        retvar, ct = self.juju('model-config', ('--reset', option,))
        return CommandComplete(NoopCondition(), ct)

    @staticmethod
    def _format_cloud_region(cloud=None, region=None):
        """Return the [[cloud/]region] in a tupple."""
        if cloud and region:
            return ('{}/{}'.format(cloud, region),)
        elif region:
            return (region,)
        elif cloud:
            raise ValueError('The cloud must be followed by a region.')
        else:
            return ()

    def get_model_defaults(self, model_key, cloud=None, region=None):
        """Return a dict with information on model-defaults for model-key.

        Giving cloud/region acts as a filter."""
        cloud_region = self._format_cloud_region(cloud, region)
        gjo_args = ('--format', 'yaml') + cloud_region + (model_key,)
        raw_yaml = self.get_juju_output('model-defaults', *gjo_args,
                                        include_e=False)
        return yaml.safe_load(raw_yaml)

    def set_model_defaults(self, model_key, value, cloud=None, region=None):
        """Set a model-defaults entry for model_key to value.

        Giving cloud/region sets the default for that region, otherwise the
        controller default is set."""
        cloud_region = self._format_cloud_region(cloud, region)
        self.juju('model-defaults',
                  cloud_region + ('{}={}'.format(model_key, value),),
                  include_e=False)

    def unset_model_defaults(self, model_key, cloud=None, region=None):
        """Unset a model-defaults entry for model_key.

        Giving cloud/region unsets the default for that region, otherwise the
        controller default is unset."""
        cloud_region = self._format_cloud_region(cloud, region)
        self.juju('model-defaults',
                  cloud_region + ('--reset', model_key), include_e=False)

    def get_agent_metadata_url(self):
        return self.get_env_option(self.agent_metadata_url)

    def set_testing_agent_metadata_url(self):
        url = self.get_agent_metadata_url()
        if 'testing' not in url:
            testing_url = url.replace('/tools', '/testing/tools')
            self.set_env_option(self.agent_metadata_url, testing_url)

    def juju(self, command, args, check=True, include_e=True,
             timeout=None, extra_env=None, suppress_err=False):
        """Run a command under juju for the current environment."""
        model = self._cmd_model(include_e, controller=False)
        return self._backend.juju(
            command, args, self.used_feature_flags, self.env.juju_home,
            model, check, timeout, extra_env, suppress_err=suppress_err)

    def expect(self, command, args=(), include_e=True,
               timeout=None, extra_env=None):
        """Return a process object that is running an interactive `command`.

        The interactive command ability is provided by using pexpect.

        :param command: String of the juju command to run.
        :param args: Tuple containing arguments for the juju `command`.
        :param include_e: Boolean regarding supplying the juju environment to
          `command`.
        :param timeout: A float that, if provided, is the timeout in which the
          `command` is run.

        :return: A pexpect.spawn object that has been called with `command` and
          `args`.

        """
        model = self._cmd_model(include_e, controller=False)
        return self._backend.expect(
            command, args, self.used_feature_flags, self.env.juju_home,
            model, timeout, extra_env)

    def controller_juju(self, command, args):
        args = ('-c', self.env.controller.name) + args
        retvar, ct = self.juju(command, args, include_e=False)
        return CommandComplete(NoopCondition(), ct)

    def get_juju_timings(self):
        timing_breakdown = []
        for ct in self._backend.juju_timings:
            timing_breakdown.append(
                {
                    'command': ct.cmd,
                    'full_args': ct.full_args,
                    'start': ct.start,
                    'end': ct.end,
                    'total_seconds': ct.total_seconds,
                }
            )
        return timing_breakdown

    def juju_async(self, command, args, include_e=True, timeout=None):
        model = self._cmd_model(include_e, controller=False)
        return self._backend.juju_async(command, args, self.used_feature_flags,
                                        self.env.juju_home, model, timeout)

    def deploy(self, charm, repository=None, to=None, series=None,
               service=None, force=False, resource=None, num=None,
               constraints=None, alias=None, bind=None, **kwargs):
        args = [charm]
        if service is not None:
            args.extend([service])
        if to is not None:
            args.extend(['--to', to])
        if series is not None:
            args.extend(['--series', series])
        if force is True:
            args.extend(['--force'])
        if resource is not None:
            args.extend(['--resource', resource])
        if num is not None:
            args.extend(['-n', str(num)])
        if constraints is not None:
            args.extend(['--constraints', constraints])
        if bind is not None:
            args.extend(['--bind', bind])
        if alias is not None:
            args.extend([alias])
        for key, value in kwargs.items():
            if isinstance(value, list):
                for item in value:
                    args.extend(['--{}'.format(key), item])
            else:
                args.extend(['--{}'.format(key), value])
        retvar, ct = self.juju('deploy', tuple(args))
        return retvar, CommandComplete(WaitAgentsStarted(), ct)

    def attach(self, service, resource):
        args = (service, resource)
        retvar, ct = self.juju('attach', args)
        return retvar, CommandComplete(NoopCondition(), ct)

    def list_resources(self, service_or_unit, details=True):
        args = ('--format', 'yaml', service_or_unit)
        if details:
            args = args + ('--details',)
        return yaml.safe_load(self.get_juju_output('list-resources', *args))

    def wait_for_resource(self, resource_id, service_or_unit, timeout=60):
        log.info('Waiting for resource. Resource id:{}'.format(resource_id))
        with self.check_timeouts():
            with self.ignore_soft_deadline():
                for _ in until_timeout(timeout):
                    resources_dict = self.list_resources(service_or_unit)
                    resources = resources_dict['resources']
                    for resource in resources:
                        if resource['expected']['resourceid'] == resource_id:
                            if (resource['expected']['fingerprint'] ==
                                    resource['unit']['fingerprint']):
                                return
                    time.sleep(.1)
                raise JujuResourceTimeout(
                    'Timeout waiting for a resource to be downloaded. '
                    'ResourceId: {} Service or Unit: {} Timeout: {}'.format(
                        resource_id, service_or_unit, timeout))

    def upgrade_charm(self, service, charm_path=None):
        args = (service,)
        if charm_path is not None:
            args = args + ('--path', charm_path)
        self.juju('upgrade-charm', args)

    def remove_service(self, service):
        self.juju('remove-application', (service,))

    @classmethod
    def format_bundle(cls, bundle_template):
        return bundle_template.format(container=cls.preferred_container())

    def deploy_bundle(self, bundle_template, timeout=_DEFAULT_BUNDLE_TIMEOUT):
        """Deploy bundle using native juju 2.0 deploy command."""
        bundle = self.format_bundle(bundle_template)
        self.juju('deploy', bundle, timeout=timeout)

    def deployer(self, bundle_template, name=None, deploy_delay=10,
                 timeout=3600):
        """Deploy a bundle using deployer."""
        bundle = self.format_bundle(bundle_template)
        args = (
            '--debug',
            '--deploy-delay', str(deploy_delay),
            '--timeout', str(timeout),
            '--config', bundle,
        )
        if name:
            args += (name,)
        e_arg = ('-e', '{}:{}'.format(
            self.env.controller.name, self.env.environment))
        args = e_arg + args
        self.juju('deployer', args, include_e=False)

    @staticmethod
    def _maas_spaces_enabled():
        return not os.environ.get("JUJU_CI_SPACELESSNESS")

    def _get_substrate_constraints(self):
        if self.env.joyent:
            # Only accept kvm packages by requiring >1 cpu core, see lp:1446264
            return 'mem=2G cpu-cores=1'
        elif self.env.maas and self._maas_spaces_enabled():
            # For now only maas support spaces in a meaningful way.
            return 'mem=2G spaces={}'.format(','.join(
                '^' + space for space in sorted(self.excluded_spaces)))
        else:
            return 'mem=2G'

    def quickstart(self, bundle_template, upload_tools=False):
        bundle = self.format_bundle(bundle_template)
        constraints = 'mem=2G'
        args = ('--constraints', constraints)
        if upload_tools:
            args = ('--upload-tools',) + args
        args = args + ('--no-browser', bundle,)
        self.juju('quickstart', args, extra_env={'JUJU': self.full_path})

    def status_until(self, timeout, start=None):
        """Call and yield status until the timeout is reached.

        Status will always be yielded once before checking the timeout.

        This is intended for implementing things like wait_for_started.

        :param timeout: The number of seconds to wait before timing out.
        :param start: If supplied, the time to count from when determining
            timeout.
        """
        with self.check_timeouts():
            with self.ignore_soft_deadline():
                yield self.get_status()
                for remaining in until_timeout(timeout, start=start):
                    yield self.get_status()

    def _wait_for_status(self, reporter, translate, exc_type=StatusNotMet,
                         timeout=1200, start=None):
        """Wait till status reaches an expected state with pretty reporting.

        Always tries to get status at least once. Each status call has an
        internal timeout of 60 seconds. This is independent of the timeout for
        the whole wait, note this means this function may be overrun.

        :param reporter: A GroupReporter instance for output.
        :param translate: A callable that takes status to make states dict.
        :param exc_type: Optional StatusNotMet subclass to raise on timeout.
        :param timeout: Optional number of seconds to wait before timing out.
        :param start: Optional time to count from when determining timeout.
        """
        status = None
        try:
            with self.check_timeouts():
                with self.ignore_soft_deadline():
                    for _ in chain([None],
                                   until_timeout(timeout, start=start)):
                        status = self.get_status()
                        states = translate(status)
                        if states is None:
                            break
                        status.raise_highest_error(ignore_recoverable=True)
                        reporter.update(states)
                    else:
                        if status is not None:
                            log.error(status.status_text)
                            status.raise_highest_error(
                                ignore_recoverable=False)
                        raise exc_type(self.env.environment, status)
        finally:
            reporter.finish()
        return status

    def wait_for_started(self, timeout=1200, start=None):
        """Wait until all unit/machine agents are 'started'."""
        reporter = GroupReporter(sys.stdout, 'started')
        return self._wait_for_status(
            reporter, Status.check_agents_started, AgentsNotStarted,
            timeout=timeout, start=start)

    def wait_for_subordinate_units(self, service, unit_prefix, timeout=1200,
                                   start=None):
        """Wait until all service units have a started subordinate with
        unit_prefix."""
        def status_to_subordinate_states(status):
            service_unit_count = status.get_service_unit_count(service)
            subordinate_unit_count = 0
            unit_states = defaultdict(list)
            for name, unit in status.service_subordinate_units(service):
                if name.startswith(unit_prefix + '/'):
                    subordinate_unit_count += 1
                    unit_states[coalesce_agent_status(unit)].append(name)
            if (subordinate_unit_count == service_unit_count and
                    set(unit_states.keys()).issubset(AGENTS_READY)):
                return None
            return unit_states
        reporter = GroupReporter(sys.stdout, 'started')
        self._wait_for_status(
            reporter, status_to_subordinate_states, AgentsNotStarted,
            timeout=timeout, start=start)

    def wait_for_version(self, version, timeout=300):
        self.wait_for(WaitVersion(version, timeout))

    def list_models(self):
        """List the models registered with the current controller."""
        self.controller_juju('list-models', ())

    def get_models(self):
        """return a models dict with a 'models': [] key-value pair.

        The server has 120 seconds to respond because this method is called
        often when tearing down a controller-less deployment.
        """
        output = self.get_juju_output(
            'list-models', '-c', self.env.controller.name, '--format', 'yaml',
            include_e=False, timeout=120)
        models = yaml.safe_load(output)
        return models

    def _get_models(self):
        """return a list of model dicts."""
        return self.get_models()['models']

    def iter_model_clients(self):
        """Iterate through all the models that share this model's controller.

        Works only if JES is enabled.
        """
        models = self._get_models()
        if not models:
            yield self
        for model in models:
            # 2.2-rc1 introduced new model listing output name/short-name.
            model_name = model.get('short-name', model['name'])
            yield self._acquire_model_client(model_name, model.get('owner'))

    def get_controller_model_name(self):
        """Return the name of the 'controller' model.

        Return the name of the environment when an 'controller' model does
        not exist.
        """
        return 'controller'

    def _acquire_model_client(self, name, owner=None):
        """Get a client for a model with the supplied name.

        If the name matches self, self is used.  Otherwise, a clone is used.
        If the owner of the model is different to the user_name of the client
        provide a fully qualified model name.

        """
        if name == self.env.environment:
            return self
        else:
            if owner and owner != self.env.user_name:
                model_name = '{}/{}'.format(owner, name)
            else:
                model_name = name
            env = self.env.clone(model_name=model_name)
            return self.clone(env=env)

    def get_model_uuid(self):
        name = self.env.environment
        model = self._cmd_model(True, False)
        output_yaml = self.get_juju_output(
            'show-model', '--format', 'yaml', model, include_e=False)
        output = yaml.safe_load(output_yaml)
        return output[name]['model-uuid']

    def get_controller_uuid(self):
        name = self.env.controller.name
        output_yaml = self.get_juju_output(
            'show-controller',
            name,
            '--format', 'yaml',
            include_e=False)
        output = yaml.safe_load(output_yaml)
        return output[name]['details']['uuid']

    def get_controller_model_uuid(self):
        output_yaml = self.get_juju_output(
            'show-model', 'controller', '--format', 'yaml', include_e=False)
        output = yaml.safe_load(output_yaml)
        return output['controller']['model-uuid']

    def get_controller_client(self):
        """Return a client for the controller model.  May return self.

        This may be inaccurate for models created using add_model
        rather than bootstrap.
        """
        return self._acquire_model_client(self.get_controller_model_name())

    def list_controllers(self):
        """List the controllers."""
        self.juju('list-controllers', (), include_e=False)

    def get_controller_endpoint(self):
        """Return the host and port of the controller leader."""
        controller = self.env.controller.name
        output = self.get_juju_output(
            'show-controller', controller, include_e=False)
        info = yaml.safe_load(output)
        endpoint = info[controller]['details']['api-endpoints'][0]
        return split_address_port(endpoint)

    def get_controller_members(self):
        """Return a list of Machines that are members of the controller.

        The first machine in the list is the leader. the remaining machines
        are followers in a HA relationship.
        """
        members = []
        status = self.get_status()
        for machine_id, machine in status.iter_machines():
            if self.get_controller_member_status(machine):
                members.append(Machine(machine_id, machine))
        if len(members) <= 1:
            return members
        # Search for the leader and make it the first in the list.
        # If the endpoint address is not the same as the leader's dns_name,
        # the members are return in the order they were discovered.
        endpoint = self.get_controller_endpoint()[0]
        log.debug('Controller endpoint is at {}'.format(endpoint))
        members.sort(key=lambda m: m.info.get('dns-name') != endpoint)
        return members

    def get_controller_leader(self):
        """Return the controller leader Machine."""
        controller_members = self.get_controller_members()
        return controller_members[0]

    @staticmethod
    def get_controller_member_status(info_dict):
        """Return the controller-member-status of the machine if it exists."""
        return info_dict.get('controller-member-status')

    def wait_for_ha(self, timeout=1200, start=None):
        """Wait for voiting to be enabled.

        May only be called on a controller client."""
        if self.env.environment != self.get_controller_model_name():
            raise ValueError('wait_for_ha requires a controller client.')
        desired_state = 'has-vote'

        def status_to_ha(status):
            status.check_agents_started()
            states = {}
            for machine, info in status.iter_machines():
                status = self.get_controller_member_status(info)
                if status is None:
                    continue
                states.setdefault(status, []).append(machine)
            if list(states.keys()) == [desired_state]:
                if len(states.get(desired_state, [])) >= 3:
                    return None
            return states

        reporter = GroupReporter(sys.stdout, desired_state)
        self._wait_for_status(reporter, status_to_ha, VotingNotEnabled,
                              timeout=timeout, start=start)
        # XXX sinzui 2014-12-04: bug 1399277 happens because
        # juju claims HA is ready when the monogo replica sets
        # are not. Juju is not fully usable. The replica set
        # lag might be 5 minutes.
        self._backend.pause(300)

    def wait_for_deploy_started(self, service_count=1, timeout=1200):
        """Wait until service_count services are 'started'.

        :param service_count: The number of services for which to wait.
        :param timeout: The number of seconds to wait.
        """
        with self.check_timeouts():
            with self.ignore_soft_deadline():
                status = None
                for remaining in until_timeout(timeout):
                    status = self.get_status()
                    if status.get_service_count() >= service_count:
                        return
                else:
                    raise ApplicationsNotStarted(self.env.environment, status)

    def wait_for_workloads(self, timeout=600, start=None):
        """Wait until all unit workloads are in a ready state."""
        def status_to_workloads(status):
            unit_states = defaultdict(list)
            for name, unit in status.iter_units():
                workload = unit.get('workload-status')
                if workload is not None:
                    state = workload['current']
                else:
                    state = 'unknown'
                unit_states[state].append(name)
            if set(('active', 'unknown')).issuperset(unit_states):
                return None
            unit_states.pop('unknown', None)
            return unit_states
        reporter = GroupReporter(sys.stdout, 'active')
        self._wait_for_status(reporter, status_to_workloads, WorkloadsNotReady,
                              timeout=timeout, start=start)

    def wait_for(self, condition, quiet=False):
        """Wait until the supplied conditions are satisfied.

        The supplied conditions must be an iterable of objects like
        WaitMachineNotPresent.
        """
        if condition.already_satisfied:
            return self.get_status()
        # iter_blocking_state must filter out all non-blocking values, so
        # there are no "expected" values for the GroupReporter.
        reporter = GroupReporter(sys.stdout, None)
        status = None
        try:
            for status in self.status_until(condition.timeout):
                status.raise_highest_error(ignore_recoverable=True)
                states = {}
                for item, state in condition.iter_blocking_state(status):
                    states.setdefault(state, []).append(item)
                if len(states) == 0:
                    return
                if not quiet:
                    reporter.update(states)
            else:
                status.raise_highest_error(ignore_recoverable=False)
        except StatusTimeout:
            pass
        finally:
            reporter.finish()
        condition.do_raise(self.model_name, status)

    def get_matching_agent_version(self, no_build=False):
        version_number = get_stripped_version_number(self.version)
        if not no_build and self.env.local:
            version_number += '.1'
        return version_number

    def upgrade_juju(self, force_version=True):
        args = ()
        if force_version:
            version = self.get_matching_agent_version(no_build=True)
            args += ('--agent-version', version)
        self._upgrade_juju(args)

    def _upgrade_juju(self, args):
        self.juju('upgrade-juju', args)

    def upgrade_mongo(self):
        self.juju('upgrade-mongo', ())

    def backup(self):
        try:
            output = self.get_juju_output('create-backup')
        except subprocess.CalledProcessError as e:
            log.info(e.output)
            raise
        log.info(output)
        backup_file_pattern = re.compile(
            '(juju-backup-[0-9-]+\.(t|tar.)gz)'.encode('ascii'))
        match = backup_file_pattern.search(output)
        if match is None:
            raise Exception("The backup file was not found in output: %s" %
                            output)
        backup_file_name = match.group(1)
        backup_file_path = os.path.abspath(backup_file_name)
        log.info("State-Server backup at %s", backup_file_path)
        return backup_file_path.decode(getpreferredencoding())

    def restore_backup(self, backup_file):
        self.juju(
            'restore-backup',
            ('-b', '--constraints', 'mem=2G', '--file', backup_file))

    def restore_backup_async(self, backup_file):
        return self.juju_async('restore-backup', ('-b', '--constraints',
                               'mem=2G', '--file', backup_file))

    def enable_ha(self):
        self.juju(
            'enable-ha', ('-n', '3', '-c', self.env.controller.name),
            include_e=False)

    def action_fetch(self, id, action=None, timeout="1m"):
        """Fetches the results of the action with the given id.

        Will wait for up to 1 minute for the action results.
        The action name here is just used for an more informational error in
        cases where it's available.
        Returns the yaml output of the fetched action.
        """
        out = self.get_juju_output("show-action-output", id, "--wait", timeout)
        status = yaml.safe_load(out)["status"]
        if status != "completed":
            name = ""
            if action is not None:
                name = " " + action
            raise Exception(
                "timed out waiting for action%s to complete during fetch" %
                name)
        return out

    def action_do(self, unit, action, *args):
        """Performs the given action on the given unit.

        Action params should be given as args in the form foo=bar.
        Returns the id of the queued action.
        """
        args = (unit, action) + args

        output = self.get_juju_output("run-action", *args)
        action_id_pattern = re.compile(
            'Action queued with id: ([a-f0-9\-]{36})')
        match = action_id_pattern.search(output)
        if match is None:
            raise Exception("Action id not found in output: %s" %
                            output)
        return match.group(1)

    def action_do_fetch(self, unit, action, timeout="1m", *args):
        """Performs given action on given unit and waits for the results.

        Action params should be given as args in the form foo=bar.
        Returns the yaml output of the action.
        """
        id = self.action_do(unit, action, *args)
        return self.action_fetch(id, action, timeout)

    def run(self, commands, applications=None, machines=None, units=None,
            use_json=True):
        args = []
        if use_json:
            args.extend(['--format', 'json'])
        if applications is not None:
            args.extend(['--application', ','.join(applications)])
        if machines is not None:
            args.extend(['--machine', ','.join(machines)])
        if units is not None:
            args.extend(['--unit', ','.join(units)])
        args.extend(commands)
        responces = self.get_juju_output('run', *args)
        if use_json:
            return json.loads(responces)
        else:
            return responces

    def list_space(self):
        return yaml.safe_load(self.get_juju_output('list-space'))

    def add_space(self, space):
        self.juju('add-space', (space),)

    def add_subnet(self, subnet, space):
        self.juju('add-subnet', (subnet, space))

    def is_juju1x(self):
        return self.version.startswith('1.')

    def _get_register_command(self, output):
        """Return register token from add-user output.

        Return the register token supplied within the output from the add-user
        command.

        """
        for row in output.split('\n'):
            if 'juju register' in row:
                command_string = row.strip().lstrip()
                command_parts = command_string.split(' ')
                return command_parts[-1]
        raise AssertionError('Juju register command not found in output')

    def add_user(self, username):
        """Adds provided user and return register command arguments.

        :return: Registration token provided by the add-user command.
        """
        output = self.get_juju_output(
            'add-user', username, '-c', self.env.controller.name,
            include_e=False)
        return self._get_register_command(output)

    def add_user_perms(self, username, models=None, permissions='login'):
        """Adds provided user and return register command arguments.

        :return: Registration token provided by the add-user command.
        """
        output = self.add_user(username)
        self.grant(username, permissions, models)
        return output

    def revoke(self, username, models=None, permissions='read'):
        if models is None:
            models = self.env.environment

        args = (username, permissions, models)

        self.controller_juju('revoke', args)

    def add_storage(self, unit, storage_type, amount="1"):
        """Add storage instances to service.

        Only type 'disk' is able to add instances.
        """
        self.juju('add-storage', (unit, storage_type + "=" + amount))

    def list_storage(self):
        """Return the storage list."""
        return self.get_juju_output('list-storage', '--format', 'json')

    def list_storage_pool(self):
        """Return the list of storage pool."""
        return self.get_juju_output('list-storage-pools', '--format', 'json')

    def create_storage_pool(self, name, provider, size):
        """Create storage pool."""
        self.juju('create-storage-pool',
                  (name, provider,
                   'size={}'.format(size)))

    def disable_user(self, user_name):
        """Disable an user"""
        self.controller_juju('disable-user', (user_name,))

    def enable_user(self, user_name):
        """Enable an user"""
        self.controller_juju('enable-user', (user_name,))

    def logout(self):
        """Logout an user"""
        self.controller_juju('logout', ())
        self.env.user_name = ''

    def _end_pexpect_session(self, session):
        """Pexpect doesn't return buffers, or handle exceptions well.
        This method attempts to ensure any relevant data is returned to the
        test output in the event of a failure, or the unexpected"""
        session.expect(pexpect.EOF)
        session.close()
        if session.exitstatus != 0:
            log.error('Buffer: {}'.format(session.buffer))
            log.error('Before: {}'.format(session.before))
            raise Exception('pexpect process exited with {}'.format(
                    session.exitstatus))

    def register_user(self, user, juju_home, controller_name=None):
        """Register `user` for the `client` return the cloned client used."""
        username = user.name
        if controller_name is None:
            controller_name = '{}_controller'.format(username)

        model = self.env.environment
        token = self.add_user_perms(username, models=model,
                                    permissions=user.permissions)
        user_client = self.create_cloned_environment(juju_home,
                                                     controller_name,
                                                     username)

        try:
            child = user_client.expect('register', (token), include_e=False)
            child.expect('(?i)password')
            child.sendline(username + '_password')
            child.expect('(?i)password')
            child.sendline(username + '_password')
            child.expect('(?i)name')
            child.sendline(controller_name)
            self._end_pexpect_session(child)
        except pexpect.TIMEOUT:
            log.error('Buffer: {}'.format(child.buffer))
            log.error('Before: {}'.format(child.before))
            raise Exception(
                'Registering user failed: pexpect session timed out')
        user_client.env.user_name = username
        return user_client

    def login_user(self, username=None, password=None):
        """Login `user` for the `client`"""
        if username is None:
            username = self.env.user_name

        self.env.user_name = username

        if password is None:
            password = '{}-{}'.format(username, 'password')

        try:
            child = self.expect(self.login_user_command,
                                (username, '-c', self.env.controller.name),
                                include_e=False)
            child.expect('(?i)password')
            child.sendline(password)
            self._end_pexpect_session(child)
        except pexpect.TIMEOUT:
            log.error('Buffer: {}'.format(child.buffer))
            log.error('Before: {}'.format(child.before))
            raise Exception(
                'FAIL Login user failed: pexpect session timed out')

    def register_host(self, host, email, password):
        child = self.expect('register', ('--no-browser-login', host),
                            include_e=False)
        try:
            child.logfile = sys.stdout
            child.expect('E-Mail:|Enter a name for this controller:')
            if child.match.group(0) == 'E-Mail:':
                child.sendline(email)
                child.expect('Password:')
                child.logfile = None
                try:
                    child.sendline(password)
                finally:
                    child.logfile = sys.stdout
                child.expect(r'Two-factor auth \(Enter for none\):')
                child.sendline()
                child.expect('Enter a name for this controller:')
            child.sendline(self.env.controller.name)
            self._end_pexpect_session(child)
        except pexpect.TIMEOUT:
            log.error('Buffer: {}'.format(child.buffer))
            log.error('Before: {}'.format(child.before))
            raise Exception(
                'Registering host failed: pexpect session timed out')

    def remove_user(self, username):
        self.juju('remove-user', (username, '-y'), include_e=False)

    def create_cloned_environment(
            self, cloned_juju_home, controller_name, user_name=None):
        """Create a cloned environment.

        If `user_name` is passed ensures that the cloned environment is updated
        to match.

        """
        user_client = self.clone(env=self.env.clone())
        user_client.env.juju_home = cloned_juju_home
        if user_name is not None and user_name != self.env.user_name:
            user_client.env.user_name = user_name
            user_client.env.environment = qualified_model_name(
                user_client.env.environment, self.env.user_name)
        user_client.env.dump_yaml(user_client.env.juju_home, None)
        # New user names the controller.
        user_client.env.controller = Controller(controller_name)
        return user_client

    def grant(self, user_name, permission, model=None):
        """Grant the user with model or controller permission."""
        if permission in self.controller_permissions:
            self.juju(
                'grant',
                (user_name, permission, '-c', self.env.controller.name),
                include_e=False)
        elif permission in self.model_permissions:
            if model is None:
                model = self.model_name
            self.juju(
                'grant',
                (user_name, permission, model, '-c', self.env.controller.name),
                include_e=False)
        else:
            raise ValueError('Unknown permission {}'.format(permission))

    def list_clouds(self, format='json'):
        """List all the available clouds."""
        return self.get_juju_output('list-clouds', '--format',
                                    format, include_e=False)

    def generate_tool(self, source_dir, stream=None):
        args = ('generate-tools', '-d', source_dir)
        if stream is not None:
            args += ('--stream', stream)
        retvar, ct = self.juju('metadata', args, include_e=False)
        return retvar, CommandComplete(NoopCondition(), ct)

    def add_cloud(self, cloud_name, cloud_file):
        retvar, ct = self.juju(
            'add-cloud', ("--replace", cloud_name, cloud_file),
            include_e=False)
        return retvar, CommandComplete(NoopCondition(), ct)

    def add_cloud_interactive(self, cloud_name, cloud):
        child = self.expect('add-cloud', include_e=False)
        try:
            child.logfile = sys.stdout
            child.expect('Select cloud type:')
            child.sendline(cloud['type'])
            child.expect('(Enter a name for your .* cloud:)|'
                         '(Select cloud type:)')
            if child.match.group(2) is not None:
                raise TypeNotAccepted('Cloud type not accepted.')
            child.sendline(cloud_name)
            if cloud['type'] == 'maas':
                child.expect('Enter the API endpoint url:')
                child.sendline(cloud['endpoint'])
            if cloud['type'] == 'manual':
                child.expect(
                    "(Enter the controller's hostname or IP address:)|"
                    "(Enter a name for your .* cloud:)")
                if child.match.group(2) is not None:
                    raise NameNotAccepted('Cloud name not accepted.')
                child.sendline(cloud['endpoint'])
            if cloud['type'] == 'openstack':
                child.expect('Enter the API endpoint url for the cloud:')
                child.sendline(cloud['endpoint'])
                child.expect(
                    "(Select one or more auth types separated by commas:)|"
                    "(Can't validate endpoint)")
                if child.match.group(2) is not None:
                    raise InvalidEndpoint()
                child.sendline(','.join(cloud['auth-types']))
                for num, (name, values) in enumerate(cloud['regions'].items()):
                    child.expect(
                        '(Enter region name:)|(Select one or more auth types'
                        ' separated by commas:)')
                    if child.match.group(2) is not None:
                        raise AuthNotAccepted('Auth was not compatible.')
                    child.sendline(name)
                    child.expect(self.REGION_ENDPOINT_PROMPT)
                    child.sendline(values['endpoint'])
                    child.expect("(Enter another region\? \(Y/n\):)|"
                                 "(Can't validate endpoint)")
                    if child.match.group(2) is not None:
                        raise InvalidEndpoint()
                    if num + 1 < len(cloud['regions']):
                        child.sendline('y')
                    else:
                        child.sendline('n')
            if cloud['type'] == 'vsphere':
                child.expect(
                    'Enter the '
                    '(vCenter address or URL|API endpoint url for the cloud):')
                child.sendline(cloud['endpoint'])
                for num, (name, values) in enumerate(cloud['regions'].items()):
                    child.expect("Enter (datacenter|region) name:|"
                                 "(?P<invalid>Can't validate endpoint)")
                    if child.match.group('invalid') is not None:
                        raise InvalidEndpoint()
                    child.sendline(name)
                    child.expect(
                        'Enter another (datacenter|region)\? \(Y/n\):')
                    if num + 1 < len(cloud['regions']):
                        child.sendline('y')
                    else:
                        child.sendline('n')

            child.expect([pexpect.EOF, "Can't validate endpoint"])
            if child.match != pexpect.EOF:
                if child.match.group(0) == "Can't validate endpoint":
                    raise InvalidEndpoint()
        except pexpect.TIMEOUT:
            raise Exception(
                'Adding cloud failed: pexpect session timed out')

    def show_controller(self, format='json'):
        """Show controller's status."""
        return self.get_juju_output('show-controller', '--format',
                                    format, include_e=False)

    def show_machine(self, machine):
        """Return data on a machine as a dict."""
        text = self.get_juju_output('show-machine', machine,
                                    '--format', 'yaml')
        return yaml.safe_load(text)

    def ssh_keys(self, full=False):
        """Give the ssh keys registered for the current model."""
        args = []
        if full:
            args.append('--full')
        return self.get_juju_output('ssh-keys', *args)

    def add_ssh_key(self, *keys):
        """Add one or more ssh keys to the current model."""
        return self.get_juju_output('add-ssh-key', *keys, merge_stderr=True)

    def remove_ssh_key(self, *keys):
        """Remove one or more ssh keys from the current model."""
        return self.get_juju_output('remove-ssh-key', *keys, merge_stderr=True)

    def import_ssh_key(self, *keys):
        """Import ssh keys from one or more identities to the current model."""
        return self.get_juju_output('import-ssh-key', *keys, merge_stderr=True)

    def list_disabled_commands(self):
        """List all the commands disabled on the model."""
        raw = self.get_juju_output('list-disabled-commands',
                                   '--format', 'yaml')
        return yaml.safe_load(raw)

    def disable_command(self, command_set, message=''):
        """Disable a command-set."""
        retvar, ct = self.juju('disable-command', (command_set, message))
        return retvar, CommandComplete(NoopCondition(), ct)

    def enable_command(self, args):
        """Enable a command-set."""
        retvar, ct = self.juju('enable-command', args)
        return CommandComplete(NoopCondition(), ct)

    def sync_tools(self, local_dir=None, stream=None, source=None):
        """Copy tools into a local directory or model."""
        args = ()
        if stream is not None:
            args += ('--stream', stream)
        if source is not None:
            args += ('--source', source)
        if local_dir is None:
            retvar, ct = self.juju('sync-tools', args)
            return retvar, CommandComplete(NoopCondition(), ct)
        else:
            args += ('--local-dir', local_dir)
            retvar, ct = self.juju('sync-tools', args, include_e=False)
            return retvar, CommandComplete(NoopCondition(), ct)

    def switch(self, model=None, controller=None):
        """Switch between models."""
        args = [x for x in [controller, model] if x]
        if not args:
            raise ValueError('No target to switch to has been given.')
        self.juju('switch', (':'.join(args),), include_e=False)


def get_local_root(juju_home, env):
    return os.path.join(juju_home, env.environment)


def bootstrap_from_env(juju_home, client):
    with temp_bootstrap_env(juju_home, client):
        client.bootstrap()


def quickstart_from_env(juju_home, client, bundle):
    with temp_bootstrap_env(juju_home, client):
        client.quickstart(bundle)


def uniquify_local(env):
    """Ensure that local environments have unique port settings.

    This allows local environments to be duplicated despite
    https://bugs.launchpad.net/bugs/1382131
    """
    if not env.local:
        return
    port_defaults = {
        'api-port': 17070,
        'state-port': 37017,
        'storage-port': 8040,
        'syslog-port': 6514,
    }
    new_config = {}
    for key, default in port_defaults.items():
        new_config[key] = env.get_option(key, default) + 1
    env.update_config(new_config)


def dump_environments_yaml(juju_home, config):
    """Dump yaml data to the environment file.

    :param juju_home: Path to the JUJU_HOME directory.
    :param config: Dictionary repersenting yaml data to dump."""
    environments_path = get_environments_path(juju_home)
    with open(environments_path, 'w') as config_file:
        yaml.safe_dump(config, config_file)


@contextmanager
def _temp_env(new_config, parent=None, set_home=True):
    """Use the supplied config as juju environment.

    This is not a fully-formed version for bootstrapping.  See
    temp_bootstrap_env.
    """
    with temp_dir(parent) as temp_juju_home:
        dump_environments_yaml(temp_juju_home, new_config)
        if set_home:
            with scoped_environ():
                os.environ['JUJU_HOME'] = temp_juju_home
                os.environ['JUJU_DATA'] = temp_juju_home
                yield temp_juju_home
        else:
            yield temp_juju_home


def jes_home_path(juju_home, dir_name):
    return os.path.join(juju_home, 'jes-homes', dir_name)


def get_cache_path(juju_home, models=False):
    if models:
        root = os.path.join(juju_home, 'models')
    else:
        root = os.path.join(juju_home, 'environments')
    return os.path.join(root, 'cache.yaml')


def make_safe_config(client):
    config = client.env.make_config_copy()
    if 'agent-version' in client.bootstrap_replaces:
        config.pop('agent-version', None)
    else:
        config['agent-version'] = client.get_matching_agent_version()
    # AFAICT, we *always* want to set test-mode to True.  If we ever find a
    # use-case where we don't, we can make this optional.
    config['test-mode'] = True
    # Explicitly set 'name', which Juju implicitly sets to env.environment to
    # ensure MAASAccount knows what the name will be.
    config['name'] = unqualified_model_name(client.env.environment)
    if config['type'] == 'local':
        config.setdefault('root-dir', get_local_root(client.env.juju_home,
                          client.env))
        # MongoDB requires a lot of free disk space, and the only
        # visible error message is from "juju bootstrap":
        # "cannot initiate replication set" if disk space is low.
        # What "low" exactly means, is unclear, but 8GB should be
        # enough.
        ensure_dir(config['root-dir'])
        check_free_disk_space(config['root-dir'], 8000000, "MongoDB files")
        if client.env.kvm:
            check_free_disk_space(
                "/var/lib/uvtool/libvirt/images", 2000000,
                "KVM disk files")
        else:
            check_free_disk_space(
                "/var/lib/lxc", 2000000, "LXC containers")
    return config


@contextmanager
def temp_bootstrap_env(juju_home, client, set_home=True, permanent=False):
    """Create a temporary environment for bootstrapping.

    This involves creating a temporary juju home directory and returning its
    location.

    :param juju_home: The current JUJU_HOME value.
    :param client: The client being prepared for bootstrapping.
    :param set_home: Set JUJU_HOME to match the temporary home in this
        context.  If False, juju_home should be supplied to bootstrap.
    :param permanent: If permanent, the environment is kept afterwards.
        Otherwise the environment is deleted when exiting the context.
    """
    new_config = {
        'environments': {client.env.environment: make_safe_config(client)}}
    # Always bootstrap a matching environment.
    jenv_path = get_jenv_path(juju_home, client.env.environment)
    if permanent:
        context = client.env.make_jes_home(
            juju_home, client.env.environment, new_config)
    else:
        context = _temp_env(new_config, juju_home, set_home)
    with context as temp_juju_home:
        if os.path.lexists(jenv_path):
            raise Exception('%s already exists!' % jenv_path)
        new_jenv_path = get_jenv_path(temp_juju_home, client.env.environment)
        # Create a symlink to allow access while bootstrapping, and to reduce
        # races.  Can't use a hard link because jenv doesn't exist until
        # partway through bootstrap.
        ensure_dir(os.path.join(juju_home, 'environments'))
        # Skip creating symlink where not supported (i.e. Windows).
        if not permanent and getattr(os, 'symlink', None) is not None:
            os.symlink(new_jenv_path, jenv_path)
        old_juju_home = client.env.juju_home
        client.env.juju_home = temp_juju_home
        try:
            yield temp_juju_home
        finally:
            if not permanent:
                # replace symlink with file before deleting temp home.
                try:
                    os.rename(new_jenv_path, jenv_path)
                except OSError as e:
                    if e.errno != errno.ENOENT:
                        raise
                    # Remove dangling symlink
                    with skip_on_missing_file():
                        os.unlink(jenv_path)
                client.env.juju_home = old_juju_home


def get_machine_dns_name(client, machine, timeout=600):
    """Wait for dns-name on a juju machine."""
    for status in client.status_until(timeout=timeout):
        try:
            return _dns_name_for_machine(status, machine)
        except KeyError:
            log.debug("No dns-name yet for machine %s", machine)


def _dns_name_for_machine(status, machine):
    host = status.status['machines'][machine]['dns-name']
    if is_ipv6_address(host):
        log.warning("Selected IPv6 address for machine %s: %r", machine, host)
    return host


class Controller:
    """Represents the controller for a model or models."""

    def __init__(self, name):
        self.name = name
        self.explicit_region = False


class GroupReporter:

    def __init__(self, stream, expected):
        self.stream = stream
        self.expected = expected
        self.last_group = None
        self.ticks = 0
        self.wrap_offset = 0
        self.wrap_width = 79

    def _write(self, string):
        self.stream.write(string)
        self.stream.flush()

    def finish(self):
        if self.last_group:
            self._write("\n")

    def update(self, group):
        if group == self.last_group:
            if (self.wrap_offset + self.ticks) % self.wrap_width == 0:
                self._write("\n")
            self._write("." if self.ticks or not self.wrap_offset else " .")
            self.ticks += 1
            return
        value_listing = []
        for value, entries in sorted(group.items()):
            if value == self.expected:
                continue
            value_listing.append('%s: %s' % (value, ', '.join(entries)))
        string = ' | '.join(value_listing)
        lead_length = len(string) + 1
        if self.last_group:
            string = "\n" + string
        self._write(string)
        self.last_group = group
        self.ticks = 0
        self.wrap_offset = lead_length if lead_length < self.wrap_width else 0


def _get_full_path(juju_path):
    """Helper to ensure a full path is used.

    If juju_path is None, ModelClient.get_full_path is used.  Otherwise,
    the supplied path is converted to absolute.
    """
    if juju_path is None:
        return ModelClient.get_full_path()
    else:
        return os.path.abspath(juju_path)


def client_from_config(config, juju_path, debug=False, soft_deadline=None):
    """Create a client from an environment's configuration.

    :param config: Name of the environment to use the config from.
    :param juju_path: Path to juju binary the client should wrap.
    :param debug=False: The debug flag for the client, False by default.
    :param soft_deadline: A datetime representing the deadline by which
        normal operations should complete.  If None, no deadline is
        enforced.
    """
    version = ModelClient.get_version(juju_path)
    if config is None:
        env = ModelClient.config_class('', {})
    else:
        env = ModelClient.config_class.from_config(config)
    full_path = _get_full_path(juju_path)
    return ModelClient(
        env, version, full_path, debug=debug, soft_deadline=soft_deadline)


def client_for_existing(juju_path, juju_data_dir, debug=False,
                        soft_deadline=None, controller_name=None,
                        model_name=None):
    """Create a client for an existing controller/model.

    :param juju_path: Path to juju binary the client should wrap.
    :param juju_data_dir: Path to the juju data directory referring the the
        controller and model.
    :param debug=False: The debug flag for the client, False by default.
    :param soft_deadline: A datetime representing the deadline by which
        normal operations should complete.  If None, no deadline is
        enforced.
    """
    version = ModelClient.get_version(juju_path)
    full_path = _get_full_path(juju_path)
    backend = ModelClient.default_backend(
        full_path, version, set(), debug=debug, soft_deadline=soft_deadline)
    if controller_name is None:
        current_controller = backend.get_active_controller(juju_data_dir)
        controller_name = current_controller
    if model_name is None:
        current_model = backend.get_active_model(juju_data_dir)
        model_name = current_model
    config = ModelClient.config_class.for_existing(
        juju_data_dir, controller_name, model_name)
    return ModelClient(
        config, version, full_path,
        debug=debug, soft_deadline=soft_deadline, _backend=backend)
