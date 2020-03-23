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

import errno
import json
import logging
import os
import pexpect
import re
import shutil
import subprocess
import sys
import time
import yaml
from collections import (
    defaultdict,
    namedtuple,
)
from contextlib import (
    contextmanager,
)
from copy import deepcopy
from itertools import chain
from locale import getpreferredencoding

from jujupy.backend import (
    JujuBackend,
)
from jujupy.configuration import (
    get_bootstrap_config_path,
    get_juju_home,
    get_selected_environment,
)
from jujupy.controller import (
    Controllers,
    ControllerConfig,
)
from jujupy.exceptions import (
    AgentsNotStarted,
    ApplicationsNotStarted,
    AuthNotAccepted,
    ControllersTimeout,
    InvalidEndpoint,
    NameNotAccepted,
    NoProvider,
    StatusNotMet,
    StatusTimeout,
    TypeNotAccepted,
    VotingNotEnabled,
    WorkloadsNotReady,
)
from jujupy.status import (
    AGENTS_READY,
    coalesce_agent_status,
    Status,
)
from jujupy.utility import (
    _dns_name_for_machine,
    JujuResourceTimeout,
    pause,
    qualified_model_name,
    skip_on_missing_file,
    split_address_port,
    temp_yaml_file,
    unqualified_model_name,
    until_timeout,
)
from jujupy.wait_condition import (
    CommandComplete,
    NoopCondition,
    WaitAgentsStarted,
    WaitMachineNotPresent,
    WaitVersion,
)

__metaclass__ = type

WIN_JUJU_CMD = os.path.join('\\', 'Progra~2', 'Juju', 'juju.exe')

CONTROLLER = 'controller'
KILL_CONTROLLER = 'kill-controller'
SYSTEM = 'system'

KVM_MACHINE = 'kvm'
LXC_MACHINE = 'lxc'
LXD_MACHINE = 'lxd'

_DEFAULT_POLL_TIMEOUT = 5
_DEFAULT_BUNDLE_TIMEOUT = 3600

log = logging.getLogger("jujupy")


def get_teardown_timeout(client):
    """Return the timeout need by the client to teardown resources."""
    if client.env.provider == 'azure':
        return 2700
    elif client.env.provider == 'gce':
        return 1200
    else:
        return 900


def parse_new_state_server_from_error(error):
    err_str = str(error)
    output = getattr(error, 'output', None)
    if output is not None:
        err_str += output
    matches = re.findall(r'Attempting to connect to (.*):22', err_str)
    if matches:
        return matches[-1]
    return None


Machine = namedtuple('Machine', ['machine_id', 'info'])


class JujuData:
    """Represents a model in a JUJU_DATA directory for juju."""

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
            self.lxd = (self._config.get('container') == 'lxd' or provider == 'lxd')
            self.kvm = (bool(self._config.get('container') == 'kvm'))
            self.maas = bool(provider == 'maas')
            self.joyent = bool(provider == 'joyent')
            self.logging_config = self._config.get('logging-config')
            self.provider_type = provider
        else:
            self.lxd = False
            self.kvm = False
            self.maas = False
            self.joyent = False
            self.logging_config = None
            self.provider_type = None
        self.credentials = {}
        self.clouds = {}
        self._cloud_name = cloud_name

    @property
    def provider(self):
        """Return the provider type for this environment.

        See get_cloud to determine the specific cloud.
        """
        try:
            return self._config['type']
        except KeyError:
            raise NoProvider('No provider specified.')

    def clone(self, model_name=None):
        config = deepcopy(self._config)
        if model_name is None:
            model_name = self.environment
        else:
            config['name'] = unqualified_model_name(model_name)
        result = JujuData(
            model_name, config, juju_home=self.juju_home,
            controller=self.controller,
            bootstrap_to=self.bootstrap_to)
        result.lxd = self.lxd
        result.kvm = self.kvm
        result.maas = self.maas
        result.joyent = self.joyent
        result.user_name = self.user_name
        result.credentials = deepcopy(self.credentials)
        result.clouds = deepcopy(self.clouds)
        result._cloud_name = self._cloud_name
        result.logging_config = self.logging_config
        result.provider_type = self.provider_type
        return result

    def set_cloud_name(self, name):
        self._cloud_name = name

    @classmethod
    def from_env(cls, env):
        juju_data = cls(env.environment, env._config, env.juju_home)
        juju_data.load_yaml()
        return juju_data

    def make_config_copy(self):
        return deepcopy(self._config)

    @contextmanager
    def make_juju_home(self, juju_home, dir_name):
        """Make a JUJU_HOME/DATA directory to avoid conflicts.

        :param juju_home: Current JUJU_HOME/DATA directory, used as a
            base path for the new directory.
        :param dir_name: Name of sub-directory to make the home in.
        """
        home_path = juju_home_path(juju_home, dir_name)
        with skip_on_missing_file():
            shutil.rmtree(home_path)
        os.makedirs(home_path)
        self.dump_yaml(home_path)
        yield home_path

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

        for key, value in new_config.items():
            if key == 'region':
                logging.warning(
                    'Using set_region to set region to "{}".'.format(value))
                self.set_region(value)
                continue
            if key == 'type':
                logging.warning('Setting type is not 2.x compatible.')
            self._config[key] = value

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
    def _from_config(cls, name):
        config, selected = get_selected_environment(name)
        if name is None:
            name = selected
        return cls(name, config)

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

    def dump_yaml(self, path):
        """Dump the configuration files to the specified path."""
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

    def find_cloud_by_host_cloud_region(self, host_cloud_region):
        self.load_yaml()
        for cloud, cloud_config in self.clouds['clouds'].items():
            if cloud_config['type'] != 'kubernetes':
                continue
            if cloud_config['host-cloud-region'] == host_cloud_region:
                return cloud
        raise LookupError(
            'No such host cloud region: {host_cloud_region}, clouds: {clouds}'.format(
                host_cloud_region=host_cloud_region,
                clouds=self.clouds['clouds'],
            )
        )

    def set_model_name(self, model_name, set_controller=True):
        if set_controller:
            self.controller.name = model_name
        self.environment = model_name
        self._config['name'] = unqualified_model_name(model_name)

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

    def get_cloud(self):
        if self._cloud_name is not None:
            return self._cloud_name
        provider = self.provider
        # Separate cloud recommended by: Juju Cloud / Credentials / BootStrap /
        # Model CLI specification
        if provider == 'ec2' and self._config['region'] == 'cn-north-1':
            return 'aws-china'
        if provider not in (
                # clouds need to handle separately.
                'maas', 'openstack', 'vsphere', 'kubernetes'
        ):
            return {
                'ec2': 'aws',
                'gce': 'google',
            }.get(provider, provider)

        if provider == 'kubernetes':
            _, k8s_base_cloud, _ = self.get_host_cloud_region()
            return k8s_base_cloud

        endpoint = ''
        if provider == 'maas':
            endpoint = self._config['maas-server']
        elif provider == 'openstack':
            endpoint = self._config['auth-url']
        elif provider == 'vsphere':
            endpoint = self._config['host']
        return self.find_endpoint_cloud(provider, endpoint)

    def get_host_cloud_region(self):
        """this is only applicable for self.provider == 'kubernetes'"""
        if self.provider != 'kubernetes':
            raise Exception("cloud type %s has to be kubernetes" % self.provider)

        def f(x):
            return [x] + x.split('/')

        cache_key = 'host-cloud-region'
        raw = getattr(self, cache_key, None)
        if raw is not None:
            return f(raw)
        try:
            raw = self._config.pop('host-cloud-region')
            setattr(self, cache_key, raw)
            return f(raw)
        except KeyError:
            raise Exception("host-cloud-region is required for kubernetes cloud")

    def get_cloud_credentials_item(self):
        cloud_name = self.get_cloud()
        cloud = self.credentials['credentials'][cloud_name]
        # cloud credential info may include defaults we need to remove
        cloud_cred = {k: v for k, v in cloud.items() if k not in ['default-region', 'default-credential']}
        (credentials_item,) = cloud_cred.items()
        return credentials_item

    def get_cloud_credentials(self):
        """Return the credentials for this model's cloud."""
        return self.get_cloud_credentials_item()[1]

    def get_option(self, key, default=None):
        return self._config.get(key, default)

    def discard_option(self, key):
        return self._config.pop(key, None)

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

    def is_cloud_provider(self):
        """Return True if the commandline cloud is a provider.

        Examples: lxd, manual
        """
        # if the commandline cloud is "lxd", "kubernetes" or "manual", the provider type
        # should match, and shortcutting get_cloud avoids pointless test
        # breakage.
        if self.provider == 'kubernetes':
            # provider is cloud type but not cloud name.
            return True

        provider_types = (
            'lxd', 'manual',
        )
        return self.provider in provider_types and self.get_cloud() in provider_types

    def _get_config_endpoint(self):
        if self.provider == 'lxd':
            return self._config.get('region', 'localhost')
        elif self.provider == 'manual':
            return self._config['bootstrap-host']
        elif self.provider == 'kubernetes':
            return self._config.get('region', None) or self.get_host_cloud_region()[2]

    def _set_config_endpoint(self, endpoint):
        if self.provider == 'lxd':
            self._config['region'] = endpoint
        elif self.provider == 'manual':
            self._config['bootstrap-host'] = endpoint
        elif self.provider == 'kubernetes':
            self._config['region'] = self.get_host_cloud_region()[2]

    def __eq__(self, other):
        if type(self) != type(other):
            return False
        if self.environment != other.environment:
            return False
        if self._config != other._config:
            return False
        if self.maas != other.maas:
            return False
        if self.bootstrap_to != other.bootstrap_to:
            return False
        return True

    def __ne__(self, other):
        return not self == other


def describe_substrate(env):
    if env.provider == 'openstack':
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


def get_stripped_version_number(version_string):
    return get_version_string_parts(version_string)[0]


def get_version_string_parts(version_string):
    # strip the series and arch from the built version.
    version_parts = version_string.split('-')
    if len(version_parts) == 4:
        # Version contains "-<patchname>", reconstruct it after the split.
        return '-'.join(version_parts[0:2]), version_parts[2], version_parts[3]
    else:
        try:
            return version_parts[0], version_parts[1], version_parts[2]
        except IndexError:
            # Possible version_string was only version (i.e. 2.0.0),
            #  namely tests.
            return version_parts


class ModelClient:
    """Wraps calls to a juju instance, associated with a single model.

    Note: A model is often called an environment (Juju 1 legacy).

    This class represents the latest Juju version.
    """

    # The environments.yaml options that are replaced by bootstrap options.
    #
    # As described in bug #1538735, default-series and --bootstrap-series must
    # match.  'default-series' should be here, but is omitted so that
    # default-series is always forced to match --bootstrap-series.
    bootstrap_replaces = frozenset(['agent-version'])

    # What feature flags have existed that CI used.
    known_feature_flags = frozenset(['actions', 'migration', 'developer-mode'])

    # What feature flags are used by this version of the juju client.
    used_feature_flags = frozenset(['migration', 'developer-mode'])

    destroy_model_command = 'destroy-model'

    supported_container_types = frozenset([KVM_MACHINE, LXC_MACHINE,
                                           LXD_MACHINE])

    default_backend = JujuBackend

    config_class = JujuData

    status_class = Status

    controllers_class = Controllers

    controller_config_class = ControllerConfig

    agent_metadata_url = 'agent-metadata-url'

    model_permissions = frozenset(['read', 'write', 'admin'])

    controller_permissions = frozenset(['login', 'add-model', 'superuser'])

    # Granting 'login' will error as a created user has that at creation.
    ignore_permissions = frozenset(['login'])

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
    _show_controller = 'show-controller'

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

    def __init__(self, env, version, full_path, juju_home=None, debug=False,
                 soft_deadline=None, _backend=None):
        """Create a new juju client.

        Required Arguments
        :param env: JujuData object representing a model in a data directory.
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
        self.env = env
        if _backend is None:
            _backend = self.default_backend(full_path, version, set(['developer-mode']), debug,
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

    def remove_machine(self, machine_ids, force=False, controller=False):
        """Remove a machine (or container).

        :param machine_ids: The ids of the machine to remove.
        :return: A WaitMachineNotPresent instance for client.wait_for.
        """
        options = ()
        if force:
            options = options + ('--force',)
        if controller:
            options = options + ('-m', 'controller',)
        self.juju('remove-machine', options + tuple(machine_ids))
        return self.make_remove_machine_condition(machine_ids)

    @staticmethod
    def get_cloud_region(cloud, region):
        if region is None:
            return cloud
        return '{}/{}'.format(cloud, region)

    def get_bootstrap_args(
            self, upload_tools, config_filename, bootstrap_series=None,
            credential=None, auto_upgrade=False, metadata_source=None,
            no_gui=False, agent_version=None, db_snap_path=None,
            db_snap_asserts_path=None, force=False, config_options=None):
        """Return the bootstrap arguments for the substrate."""
        cloud_region = self.get_cloud_region(self.env.get_cloud(),
                                             self.env.get_region())
        args = [
            # Note cloud_region before controller name
            cloud_region, self.env.environment,
            '--config', config_filename,
        ]
        if self.env.provider == 'kubernetes':
            return tuple(args)

        args += [
            '--constraints', self._get_substrate_constraints(),
            '--default-model', self.env.environment
        ]
        if force:
            args.extend(['--force'])
        if config_options:
            args.extend(['--config', config_options])
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
        if db_snap_path and db_snap_asserts_path:
            args.extend(['--db-snap', db_snap_path,
                         '--db-snap-asserts', db_snap_asserts_path])
        return tuple(args)

    def add_model(self, env, cloud_region=None, use_bootstrap_config=True):
        """Add a model to this model's controller and return its client.

        :param env: Either a class representing the new model/environment
            or the name of the new model/environment which will then be
            otherwise identical to the current model/environment."""
        if not isinstance(env, JujuData):
            env = self.env.clone(env)
        model_client = self.clone(env)
        if use_bootstrap_config:
            with model_client._bootstrap_config() as config_file:
                self._add_model(env.environment, config_file, cloud_region=cloud_region)
        else:
            # This allows the model to inherit model defaults
            self._add_model(env.environment, None, cloud_region=cloud_region)
        # Make sure we track this in case it needs special cleanup (i.e. using
        # an existing controller).
        self._backend.track_model(model_client)
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
            'audit-log-capture-args',
            'audit-log-max-size',
            'audit-log-max-backups',
            'auditing-enabled',
            'audit-log-exclude-methods',
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
            # TODO(thumper): remove max-logs-age and max-logs-size in 2.7 branch.
            'max-logs-age',
            'max-logs-size',
            'max-txn-log-size',
            'model-logs-size',
            'name',
            'password',
            'private-key',
            'project-id',
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
            'host-cloud-region',
        })

    @contextmanager
    def _bootstrap_config(self, mongo_memory_profile=None, caas_image_repo=None):
        cfg = self.make_model_config()
        if mongo_memory_profile:
            cfg['mongo-memory-profile'] = mongo_memory_profile
        if caas_image_repo:
            cfg['caas-image-repo'] = caas_image_repo
        with temp_yaml_file(cfg) as config_filename:
            yield config_filename

    def _check_bootstrap(self):
        if self.env.environment != self.env.controller.name:
            raise AssertionError(
                'Controller and environment names should not vary (yet)')

    def update_user_name(self):
        self.env.user_name = 'admin'

    def bootstrap(self, upload_tools=False, bootstrap_series=None,
                  credential=None, auto_upgrade=False, metadata_source=None,
                  no_gui=False, agent_version=None, db_snap_path=None,
                  db_snap_asserts_path=None, mongo_memory_profile=None, caas_image_repo=None, force=False,
                  config_options=None):
        """Bootstrap a controller."""
        self._check_bootstrap()
        with self._bootstrap_config(
                mongo_memory_profile, caas_image_repo,
        ) as config_filename:
            args = self.get_bootstrap_args(
                upload_tools=upload_tools,
                config_filename=config_filename,
                bootstrap_series=bootstrap_series,
                credential=credential,
                auto_upgrade=auto_upgrade,
                metadata_source=metadata_source,
                no_gui=no_gui,
                agent_version=agent_version,
                db_snap_path=db_snap_path,
                db_snap_asserts_path=db_snap_asserts_path,
                force=force,
                config_options=config_options
            )
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
                upload_tools=upload_tools,
                config_filename=config_filename,
                bootstrap_series=bootstrap_series,
                credential=None,
                auto_upgrade=auto_upgrade,
                metadata_source=metadata_source,
                no_gui=no_gui,
            )
            self.update_user_name()
            with self.juju_async('bootstrap', args, include_e=False):
                yield
                log.info('Waiting for bootstrap of {}.'.format(
                    self.env.environment))

    def _add_model(self, model_name, config_file, cloud_region=None):
        explicit_region = self.env.controller.explicit_region
        region_args = (cloud_region,) if cloud_region else ()
        if explicit_region and not region_args:
            credential_name = self.env.get_cloud_credentials_item()[0]
            cloud_region = self.get_cloud_region(self.env.get_cloud(),
                                                 self.env.get_region())
            region_args = (cloud_region, '--credential', credential_name)
        config_args = ('--config', config_file) if config_file is not None else ()
        self.controller_juju('add-model', (model_name,) + region_args + config_args)

    def destroy_model(self):
        exit_status, _ = self.juju(
            'destroy-model',
            ('{}:{}'.format(self.env.controller.name, self.env.environment),
             '-y', '--destroy-storage',),
            include_e=False, timeout=get_teardown_timeout(self))
        # Ensure things don't get confused at teardown time (i.e. if using an
        #  existing controller)
        self._backend.untrack_model(self)
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

    def destroy_controller(self, all_models=False, destroy_storage=False, release_storage=False):
        """Destroy a controller and its models. Soft kill option.

        :param all_models: If true will attempt to destroy all the
            controller's models as well.
        :raises: subprocess.CalledProcessError if the operation fails.
        :return: Tuple: Subprocess's exit code, CommandComplete object.
        """
        args = (self.env.controller.name, '-y')
        if all_models:
            args += ('--destroy-all-models',)
        if destroy_storage:
            args += ('--destroy-storage',)
        if release_storage:
            args += ('--release-storage',)
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
            self.destroy_controller(all_models=True, destroy_storage=True)
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
        # Get the model here first, before using the get_raw_juju_output, so
        # we can ensure that the model exists on the controller.
        return self.get_raw_juju_output(command, model, *args, **kwargs)

    def get_raw_juju_output(self, command, model, *args, **kwargs):
        """Call a juju command without calling a model for it's values first.
        Passing in the model, ensures that we target the juju command with the
        right model. For global commands that aren't model specific, then you
        can pass None.

        Sub process will be called as 'juju <command> <args> <kwargs>'. Note
        that <command> may be a space delimited list of arguments. The -e
        <environment> flag will be placed after <command> and before args.
        """
        pass_kwargs = dict(
            (k, kwargs[k]) for k in kwargs if k in ['timeout', 'merge_stderr'])
        return self._backend.get_juju_output(
            command, args, self.used_feature_flags, self.env.juju_home,
            model, user_name=self.env.user_name, **pass_kwargs)

    def show_status(self):
        """Print the status to output."""
        self.juju(self._show_status, ('--format', 'yaml'))

    def get_status(self, timeout=60, raw=False, controller=False, *args):
        """Get the current status as a jujupy.status.Status object."""
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

    def get_controllers(self, timeout=60):
        """Get the current controller information as a dict."""
        for ignored in until_timeout(timeout):
            try:
                return self.controllers_class.from_text(
                    self.get_juju_output(
                        self._show_controller, '--format', 'yaml',
                        include_e=False,
                    ).decode('utf-8'),
                )
            except subprocess.CalledProcessError:
                pass
        raise ControllersTimeout(
            'Timed out waiting for juju show-controllers to succeed')

    def get_controller_config(self, controller_name, timeout=60):
        """Get controller config."""
        for ignored in until_timeout(timeout):
            try:
                return self.controller_config_class.from_text(
                    self.get_juju_output(
                        'controller-config',
                        '--controller', controller_name,
                        '--format', 'yaml',
                        include_e=False,
                    ).decode('utf-8'),
                )
            except subprocess.CalledProcessError:
                pass
        raise ControllersTimeout(
            'Timed out waiting for juju controller-config to succeed')

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
               constraints=None, alias=None, bind=None, wait_timeout=None, **kwargs):
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
        # Unfortunately some times we need to up the wait condition timeout if
        # we're deploying a complex set of machines/containers.
        return retvar, CommandComplete(WaitAgentsStarted(wait_timeout), ct)

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

    def upgrade_charm(self, service, charm_path=None, resvision=None):
        args = (service,)
        if charm_path is not None:
            args = args + ('--path', charm_path)
        if resvision is not None:
            args = args + ('--revision', resvision)
        self.juju('upgrade-charm', args)

    def remove_application(self, service):
        self.juju('remove-application', (service,))

    @classmethod
    def format_bundle(cls, bundle_template):
        return bundle_template.format(container=cls.preferred_container())

    def deploy_bundle(self, bundle_template, timeout=_DEFAULT_BUNDLE_TIMEOUT, static_bundle=False):
        """Deploy bundle using native juju 2.0 deploy command.

        :param static_bundle: render `bundle_template` if it's not static
        """
        if not static_bundle:
            bundle_template = self.format_bundle(bundle_template)
        self.juju('deploy', bundle_template, timeout=timeout)

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
            return 'cores=1'
        elif self.env.maas and self._maas_spaces_enabled():
            # For now only maas support spaces in a meaningful way.
            return 'spaces={}'.format(','.join(
                '^' + space for space in sorted(self.excluded_spaces)))
        elif self.env.lxd:
            # LXD should be constrained by memory when running in HA, otherwise
            # mongo just eats everything.
            return 'mem=6G'
        else:
            return ''

    def quickstart(self, bundle_template, upload_tools=False):
        bundle = self.format_bundle(bundle_template)
        args = ('--no-browser', bundle,)
        if upload_tools:
            args = ('--upload-tools',) + args
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
                        time.sleep(_DEFAULT_POLL_TIMEOUT)
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
        """Iterate through all the models that share this model's controller"""
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

    def wait_for_ha(self, timeout=1200, start=None, quorum=3):
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
                if len(states.get(desired_state, [])) >= quorum:
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
                    time.sleep(_DEFAULT_POLL_TIMEOUT)
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
                time.sleep(_DEFAULT_POLL_TIMEOUT)
            else:
                status.raise_highest_error(ignore_recoverable=False)
        except StatusTimeout:
            pass
        finally:
            reporter.finish()
        condition.do_raise(self.model_name, status)

    def get_matching_agent_version(self):
        version_number = get_stripped_version_number(self.version)
        return version_number

    def upgrade_juju(self, force_version=True):
        args = ()
        if force_version:
            version = self.get_matching_agent_version()
            args += ('--agent-version', version)
        self._upgrade_juju(args)

    def _upgrade_juju(self, args):
        self.juju('upgrade-juju', args)

    def upgrade_mongo(self):
        self.juju('upgrade-mongo', ())

    def backup(self):
        try:
            # merge_stderr is required for creating a backup
            output = self.get_juju_output('create-backup', merge_stderr=True)
        except subprocess.CalledProcessError as e:
            log.error('error creating backup {}'.format(e.output))
            raise
        log.info('backup file {}'.format(output))
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
            ('--file', backup_file))

    def restore_backup_async(self, backup_file):
        return self.juju_async('restore-backup', ('--file', backup_file))

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
            action_name = '' if not action else ' "{}"'.format(action)
            raise Exception(
                'Timed out waiting for action{} to complete during fetch '
                'with status: {}.'.format(action_name, status))
        return out

    def action_do(self, unit, action, *args):
        """Performs the given action on the given unit.

        Action params should be given as args in the form foo=bar.
        Returns the id of the queued action.
        """
        args = (unit, action) + args

        output = self.get_juju_output("run-action", *args)
        action_id_pattern = re.compile('Action queued with id: "([0-9]+)"')
        match = action_id_pattern.search(output)
        if match is None:
            raise Exception("Action id not found in output: {}".format(output))
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
        responses = self.get_juju_output('run', *args)
        if use_json:
            return json.loads(responses)
        else:
            return responses

    def list_space(self):
        return yaml.safe_load(self.get_juju_output('list-space'))

    def add_space(self, space):
        self.juju('add-space', (space), )

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
        user_client.env.user_name = username
        register_user_interactively(user_client, token, controller_name)
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
        user_client.env.dump_yaml(user_client.env.juju_home)
        # New user names the controller.
        user_client.env.controller = Controller(controller_name)
        return user_client

    def grant(self, user_name, permission, model=None):
        """Grant the user with model or controller permission."""
        if permission in self.ignore_permissions:
            log.info('Ignoring permission "{}".'.format(permission))
            return
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
            match = child.expect([
                'Enter a name for your .* cloud:',
                'Select cloud type:'
            ])
            if match == 1:
                raise TypeNotAccepted('Cloud type not accepted.')
            child.sendline(cloud_name)
            if cloud['type'] == 'maas':
                self.handle_maas(child, cloud)
            if cloud['type'] == 'manual':
                self.handle_manual(child, cloud)
            if cloud['type'] == 'openstack':
                self.handle_openstack(child, cloud)
            if cloud['type'] == 'vsphere':
                self.handle_vsphere(child, cloud)
            match = child.expect(["Do you ONLY want to add cloud", "Can't validate endpoint", pexpect.EOF])
            if match == 0:
                child.sendline("y")
            if match == 1:
                raise InvalidEndpoint()
            if match == 2:
                # The endpoint was validated and there isn't a controller to
                # ask about.
                return
            child.expect([pexpect.EOF, "Can't validate endpoint"])
            if child.match != pexpect.EOF:
                if child.match.group(0) == "Can't validate endpoint":
                    raise InvalidEndpoint()
        except pexpect.TIMEOUT:
            raise Exception(
                'Adding cloud failed: pexpect session timed out')

    def handle_maas(self, child, cloud):
        match = child.expect([
            'Enter the API endpoint url:',
            'Enter a name for your .* cloud:',
        ])
        if match == 1:
            raise NameNotAccepted('Cloud name not accepted.')
        child.sendline(cloud['endpoint'])

    def handle_manual(self, child, cloud):
        match = child.expect([
            "Enter a name for your .* cloud:",
            "Enter the ssh connection string for controller",
            "Enter the controller's hostname or IP address:",
            pexpect.EOF
        ])
        if match == 0:
            raise NameNotAccepted('Cloud name not accepted.')
        else:
            child.sendline(cloud['endpoint'])

    def handle_openstack(self, child, cloud):
        match = child.expect([
            'Enter the API endpoint url for the cloud',
            "Enter a name for your .* cloud:"
        ])
        if match == 1:
            raise NameNotAccepted('Cloud name not accepted.')
        child.sendline(cloud['endpoint'])
        match = child.expect([
            "Enter a path to the CA certificate for your cloud if one is required to access it",
            "Can't validate endpoint:",
        ])
        if match == 1:
            raise InvalidEndpoint()
        child.sendline("")
        match = child.expect("Select one or more auth types separated by commas:")
        if match == 0:
            child.sendline(','.join(cloud['auth-types']))
        for num, (name, values) in enumerate(cloud['regions'].items()):
            match = child.expect([
                'Enter region name:',
                'Select one or more auth types separated by commas:',
            ])
            if match == 1:
                raise AuthNotAccepted('Auth was not compatible.')
            child.sendline(name)
            child.expect(self.REGION_ENDPOINT_PROMPT)
            child.sendline(values['endpoint'])
            match = child.expect([
                "Enter another region\? \([yY]/[nN]\):",
                "Can't validate endpoint"
            ])
            if match == 1:
                raise InvalidEndpoint()
            if num + 1 < len(cloud['regions']):
                child.sendline('y')
            else:
                child.sendline('n')

    def handle_vsphere(self, child, cloud):
        match = child.expect(["Enter a name for your .* cloud:",
                              'Enter the (vCenter address or URL|API endpoint url for the cloud \[\]):'])
        if match == 0:
            raise NameNotAccepted('Cloud name not accepted.')
        if match == 1:
            child.sendline(cloud['endpoint'])

        for num, (name, values) in enumerate(cloud['regions'].items()):
            match = child.expect([
                "Enter datacenter name",
                "Enter region name",
                "Can't validate endpoint"
            ])
            if match == 2:
                raise InvalidEndpoint()
            child.sendline(name)
            child.expect(
                'Enter another (datacenter|region)\? \([yY]/[nN]\):')
            if num + 1 < len(cloud['regions']):
                child.sendline('y')
            else:
                child.sendline('n')

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


class IaasClient:
    """IaasClient defines a client that can interact with IAAS setup directly.
    """

    def __init__(self, client):
        self.client = client
        self.juju_home = self.client.env.juju_home

    def add_model(self, model_name):
        return self.client.add_model(env=self.client.env.clone(model_name))

    @property
    def is_cluster_healthy(self):
        return True


def register_user_interactively(client, token, controller_name):
    """Register a user with the supplied token and controller name.

    :param client: ModelClient on which to register the user (using the models
      controller.)
    :param token: Token string to use when registering.
    :param controller_name: String to use when naming the controller.
    """
    try:
        child = client.expect('register', (token), include_e=False)
        child.expect('Enter a new password:')
        child.sendline(client.env.user_name + '_password')
        child.expect('Confirm password:')
        child.sendline(client.env.user_name + '_password')
        child.expect('Enter a name for this controller \[.*\]:')
        child.sendline(controller_name)
        client._end_pexpect_session(child)
    except pexpect.TIMEOUT:
        log.error('Buffer: {}'.format(child.buffer))
        log.error('Before: {}'.format(child.before))
        raise Exception(
            'Registering user failed: pexpect session timed out')


def juju_home_path(juju_home, dir_name):
    return os.path.join(juju_home, 'juju-homes', dir_name)


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
    # Pass the logging config into the yaml file
    if client.env.logging_config is not None:
        config['logging-config'] = client.env.logging_config

    return config


@contextmanager
def temp_bootstrap_env(juju_home, client):
    """Create a temporary environment for bootstrapping.

    This involves creating a temporary juju home directory and returning its
    location.

    :param juju_home: The current JUJU_HOME value.
    :param client: The client being prepared for bootstrapping.
    :param set_home: Set JUJU_HOME to match the temporary home in this
        context.  If False, juju_home should be supplied to bootstrap.
    """
    # Always bootstrap a matching environment.
    context = client.env.make_juju_home(juju_home, client.env.environment)
    with context as temp_juju_home:
        client.env.juju_home = temp_juju_home
        yield temp_juju_home


def get_machine_dns_name(client, machine, timeout=600):
    """Wait for dns-name on a juju machine."""
    for status in client.status_until(timeout=timeout):
        try:
            return _dns_name_for_machine(status, machine)
        except KeyError:
            log.debug("No dns-name yet for machine %s", machine)


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
