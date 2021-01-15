import errno
import json
import logging
import os
import re
import socket
import subprocess
import sys
from contextlib import contextmanager
from datetime import (
    datetime,
    timedelta,
)
from time import (
    sleep,
    time,
)

from jujupy.utility import (
    ensure_deleted,
    ensure_dir,
    get_timeout_path,
    get_unit_public_ip,
    is_ipv6_address,
    print_now,
    qualified_model_name,
    quote,
    scoped_environ,
    skip_on_missing_file,
    temp_dir,
    temp_yaml_file,
    until_timeout
)

# Imported for other call sites to use.
__all__ = [
    'ensure_deleted',
    'ensure_dir',
    'get_timeout_path',
    'get_unit_public_ip',
    'qualified_model_name',
    'quote',
    'scoped_environ',
    'skip_on_missing_file',
    'temp_dir',
    'temp_yaml_file',
]

# Equivalent of socket.EAI_NODATA when using windows sockets
# <https://msdn.microsoft.com/ms740668#WSANO_DATA>
WSANO_DATA = 11004

TEST_MODEL = 'test-tmp-env'

log = logging.getLogger("utility")


class PortTimeoutError(Exception):
    pass


class LoggedException(BaseException):
    """Raised in place of an exception that has already been logged.

    This is a wrapper to avoid double-printing real Exceptions while still
    unwinding the stack appropriately.
    """

    def __init__(self, exception):
        self.exception = exception


class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


def _clean_dir(maybe_dir):
    """Pseudo-type that validates an argument to be a clean directory path.

    For safety, this function will not attempt to remove existing directory
    contents but will just report a warning.
    """
    try:
        contents = os.listdir(maybe_dir)
    except OSError as e:
        if e.errno == errno.ENOENT:
            # we don't raise this error due to tests abusing /tmp/logs
            logging.warning('Not a directory {}'.format(maybe_dir))
        if e.errno == errno.EEXIST:
            logging.warning('Directory {} already exists'.format(maybe_dir))
    else:
        if contents and contents != ["empty"]:
            logging.warning(
                'Directory {!r} has existing contents.'.format(maybe_dir))
    return maybe_dir


def as_literal_address(address):
    """Returns address in form suitable for embedding in URL or similar.

    In practice, this just puts square brackets round IPv6 addresses which
    avoids conflict with port seperators and other uses of colons.
    """
    if is_ipv6_address(address):
        return address.join("[]")
    return address


def wait_for_port(host, port, closed=False, timeout=30):
    family = socket.AF_INET6 if is_ipv6_address(host) else socket.AF_INET
    for remaining in until_timeout(timeout):
        try:
            addrinfo = socket.getaddrinfo(host, port, family,
                                          socket.SOCK_STREAM)
        except socket.error as e:
            if e.errno not in (socket.EAI_NODATA, WSANO_DATA):
                raise
            if closed:
                return
            else:
                continue
        sockaddr = addrinfo[0][4]
        # Treat Azure messed-up address lookup as a closed port.
        if sockaddr[0] == '0.0.0.0':
            if closed:
                return
            else:
                continue
        conn = socket.socket(*addrinfo[0][:3])
        conn.settimeout(max(remaining or 0, 5))
        try:
            conn.connect(sockaddr)
        except socket.timeout:
            if closed:
                return
        except socket.error as e:
            if e.errno not in (errno.ECONNREFUSED, errno.ENETUNREACH,
                               errno.ETIMEDOUT, errno.EHOSTUNREACH):
                raise
            if closed:
                return
        except socket.gaierror as e:
            print_now(str(e))
        except Exception as e:
            print_now('Unexpected {!r}: {}'.format(type(e), e))
            raise
        else:
            conn.close()
            if not closed:
                return
            sleep(1)
    raise PortTimeoutError('Timed out waiting for port.')


def get_revision_build(build_info):
    for action in build_info['actions']:
        if 'parameters' in action:
            for parameter in action['parameters']:
                if parameter['name'] == 'revision_build':
                    return parameter['value']


def get_winrm_certs():
    """"Returns locations of key and cert files for winrm in cloud-city."""
    home = os.environ['HOME']
    return (
        os.path.join(home, 'cloud-city/winrm_client_cert.key'),
        os.path.join(home, 'cloud-city/winrm_client_cert.pem'),
    )


def s3_cmd(params, drop_output=False):
    s3cfg_path = os.path.join(
        os.environ['HOME'], 'cloud-city/juju-qa.s3cfg')
    command = ['s3cmd', '-c', s3cfg_path, '--no-progress'] + params
    if drop_output:
        return subprocess.check_call(
            command, stdout=open('/dev/null', 'w'))
    else:
        return subprocess.check_output(command)


def _get_test_name_from_filename():
    try:
        calling_file = sys._getframe(2).f_back.f_globals['__file__']
        return os.path.splitext(os.path.basename(calling_file))[0]
    except:  # noqa: E722
        return 'unknown_test'


def generate_default_clean_dir(temp_env_name):
    """Creates a new unique directory for logging and returns name"""
    logging.debug('Environment {}'.format(temp_env_name))
    test_name = temp_env_name.split('-')[0]
    timestamp = datetime.now().strftime("%Y%m%d%H%M%S")
    log_dir = os.path.join('/tmp', test_name, 'logs', timestamp)

    try:
        os.makedirs(log_dir)
        logging.info('Created logging directory {}'.format(log_dir))
    except OSError as e:
        if e.errno == errno.EEXIST:
            logging.warn('"Directory {} already exists'.format(log_dir))
        else:
            raise ('Failed to create logging directory: {} ' +
                   log_dir +
                   '. Please specify empty folder or try again')
    return log_dir


def _generate_default_temp_env_name():
    """Creates a new unique name for environment and returns the name"""
    # we need to sanitize the name
    timestamp = datetime.now().strftime("%Y%m%d%H%M%S")
    test_name = re.sub('[^a-zA-Z]', '', _get_test_name_from_filename())
    return '{}-{}-temp-env'.format(test_name, timestamp)


def _to_deadline(timeout):
    return datetime.utcnow() + timedelta(seconds=int(timeout))


def add_arg_juju_bin(parser):
    parser.add_argument('juju_bin', nargs='?',
                        help='Full path to the Juju binary. By default, this'
                             ' will use $PATH/juju',
                        default=None)


def add_basic_testing_arguments(
        parser, using_jes=False, deadline=True, env=True, existing=True):
    """Returns the parser loaded with basic testing arguments.

    The basic testing arguments, used in conjuction with boot_context ensures
    a test can be run in any supported substrate in parallel.

    This helper adds 4 positional arguments that defines the minimum needed
    to run a test script.

    These arguments (env, juju_bin, logs, temp_env_name) allow you to specify
    specifics for which env, juju binary, which folder for logging and an
    environment name for your test respectively.

    There are many optional args that either update the env's config or
    manipulate the juju command line options to test in controlled situations
    or in uncommon substrates: --debug, --verbose, --agent-url, --agent-stream,
    --series, --bootstrap-host, --machine, --keep-env. If not using_jes, the
    --upload-tools arg will also be added.

    :param parser: an ArgumentParser.
    :param using_jes: whether args should be tailored for JES testing.
    :param deadline: If true, support the --timeout option and convert to a
        deadline.
    :param existing: If true will supply the 'existing' argument to allow
        running on an existing bootstrapped controller.
    """

    # Optional postional arguments
    if env:
        parser.add_argument(
            'env', nargs='?',
            help='The juju environment to base the temp test environment on.',
            default='lxd')
    add_arg_juju_bin(parser)
    parser.add_argument('logs', nargs='?', type=_clean_dir,
                        help='A directory in which to store logs. By default,'
                             ' this will use the current directory',
                        default=None)
    parser.add_argument('temp_env_name', nargs='?',
                        help='A temporary test environment name. By default, '
                             ' this will generate an enviroment name using the'
                             ' timestamp and testname. '
                             ' test_name_timestamp_temp_env',
                        default=_generate_default_temp_env_name())

    # Optional keyword arguments.
    parser.add_argument('--debug', action='store_true',
                        help='Pass --debug to Juju.')
    parser.add_argument('--verbose', action='store_const',
                        default=logging.INFO, const=logging.DEBUG,
                        help='Verbose test harness output.')
    parser.add_argument('--region', help='Override environment region.')
    parser.add_argument('--to', default=None,
                        help='Place the controller at a location.')
    parser.add_argument('--agent-url', action='store', default=None,
                        help='URL for retrieving agent binaries.')
    parser.add_argument('--agent-stream', action='store', default=None,
                        help='Stream for retrieving agent binaries.')
    parser.add_argument('--series', action='store', default=None,
                        help='Name of the Ubuntu series to use.')
    parser.add_argument('--arch', action='store', default=None,
                        help='Name of the architecture to use.')
    if not using_jes:
        parser.add_argument('--upload-tools', action='store_true',
                            help='upload local version of tools to bootstrap.')
    parser.add_argument('--bootstrap-host',
                        help='The host to use for bootstrap.')
    parser.add_argument('--machine', help='A machine to add or when used with '
                                          'KVM based MaaS, a KVM image to '
                                          'start.',
                        action='append', default=[])
    parser.add_argument('--keep-env', action='store_true',
                        help='Keep the Juju environment after the test'
                             ' completes.')
    parser.add_argument('--logging-config',
                        help="Override logging configuration for a "
                             "deployment.",
                        default="<root>=INFO;unit=INFO")
    parser.add_argument('--juju-home', help="Directory of juju home. It is not"
                                            " used during integration test "
                                            "runs. One can override this arg "
                                            "for local runs.", default=None)

    if existing:
        parser.add_argument(
            '--existing',
            action='store',
            default=None,
            const='current',
            nargs='?',
            help='Test using an existing bootstrapped controller. '
                 'If no controller name is provided defaults to using the '
                 'current selected controller.')
    if deadline:
        parser.add_argument('--timeout', dest='deadline', type=_to_deadline,
                            help="The script timeout, in seconds.")
    return parser


# suppress nosetests
add_basic_testing_arguments.__test__ = False


def configure_logging(log_level, logger=None):
    format = '%(asctime)s %(levelname)s %(message)s'
    datefmt = '%Y-%m-%d %H:%M:%S'
    logging.basicConfig(
        level=log_level, format=format,
        datefmt=datefmt)
    if logger:
        formatter = logging.Formatter(fmt=format, datefmt=datefmt)
        for handler in logger.handlers:
            handler.setLevel(log_level)
            handler.setFormatter(formatter)


def get_candidates_path(root_dir):
    return os.path.join(root_dir, 'candidate')


# GZ 2015-10-15: Paths returned in filesystem dependent order, may want sort?
def find_candidates(root_dir, find_all=False):
    return (path for path, buildvars in _find_candidates(root_dir, find_all))


def find_latest_branch_candidates(root_dir):
    """Return a list of one candidate per branch.

    :param root_dir: The root directory to find candidates from.
    """
    candidates = []
    for path, buildvars_path in _find_candidates(root_dir, find_all=False,
                                                 artifacts=True):
        with open(buildvars_path) as buildvars_file:
            buildvars = json.load(buildvars_file)
            candidates.append(
                (buildvars['branch'], int(buildvars['revision_build']), path))
    latest = dict(
        (branch, (path, build)) for branch, build, path in sorted(candidates))
    return latest.values()


def _find_candidates(root_dir, find_all=False, artifacts=False):
    candidates_path = get_candidates_path(root_dir)
    a_week_ago = time() - timedelta(days=7).total_seconds()
    for candidate_dir in os.listdir(candidates_path):
        if candidate_dir.endswith('-artifacts') != artifacts:
            continue
        candidate_path = os.path.join(candidates_path, candidate_dir)
        buildvars = os.path.join(candidate_path, 'buildvars.json')
        try:
            stat = os.stat(buildvars)
        except OSError as e:
            if e.errno in (errno.ENOENT, errno.ENOTDIR):
                continue
            raise
        if not find_all and stat.st_mtime < a_week_ago:
            continue
        yield candidate_path, buildvars


def get_deb_arch():
    """Get the debian machine architecture."""
    return subprocess.check_output(['dpkg', '--print-architecture']).strip()


def extract_deb(package_path, directory):
    """Extract a debian package to a specified directory."""
    subprocess.check_call(['dpkg', '-x', package_path, directory])


def run_command(command, dry_run=False, verbose=False):
    """Optionally execute a command and maybe print the output."""
    if verbose:
        print_now('Executing: {}'.format(command))
    if not dry_run:
        output = subprocess.check_output(command)
        if verbose:
            print_now(output)


def log_and_wrap_exception(logger, exc):
    """Record exc details to logger and return wrapped in LoggedException."""
    logger.exception(exc)
    stdout = getattr(exc, 'output', None)
    stderr = getattr(exc, 'stderr', None)
    if stdout or stderr:
        logger.info('Output from exception:\nstdout:\n%s\nstderr:\n%s',
                    stdout, stderr)
    return LoggedException(exc)


@contextmanager
def logged_exception(logger):
    """\
    Record exceptions in managed context to logger and reraise LoggedException.

    Note that BaseException classes like SystemExit, GeneratorExit and
    LoggedException itself are not wrapped, except for KeyboardInterrupt.
    """
    try:
        yield
    except (Exception, KeyboardInterrupt) as e:
        raise log_and_wrap_exception(logger, e)


def assert_dict_is_subset(sub_dict, super_dict):
    """Assert that every item in the sub_dict is in the super_dict.

    :raises JujuAssertionError: when sub_dict items are missing.
    :return: True when when sub_dict is a subset of super_dict
    """
    if not is_subset(sub_dict, super_dict):
        raise JujuAssertionError(
            'Found: {} \nExpected: {}'.format(super_dict, sub_dict))
    return True

def is_subset(subset, superset):
    """ Recursively check that subset is indeed a subset of superset """
    if isinstance(subset, dict):
        return all(key in superset and is_subset(val, superset[key]) for key, val in iter(subset.items()))
    if isinstance(subset, list) or isinstance(subset, set):
        return all(any(is_subset(subitem, superitem) for superitem in superset) for subitem in subset)
    return subset == superset


def add_model(client):
    """Adds a model to the current juju environment then destroys it.

    Will raise an exception if the Juju does not deselect the current model.
    :param client: Jujupy ModelClient object
    """
    log.info('Adding model "{}" to current controller'.format(TEST_MODEL))
    new_client = client.add_model(TEST_MODEL)
    new_model = get_current_model(new_client)
    if new_model == TEST_MODEL:
        log.info('Current model and newly added model match')
    else:
        error = ('Juju failed to switch to new model after creation. '
                 'Expected {} got {}'.format(TEST_MODEL, new_model))
        raise JujuAssertionError(error)
    return new_client


def get_current_model(client):
    """Gets the current model from Juju's list-models command.

    :param client: Jujupy ModelClient object
    :return: String name of current model
    """
    raw = list_models(client)
    try:
        return raw['current-model']
    except KeyError:
        log.warning('No model is currently selected.')
        return None


def list_models(client):
    """List models.
    :param client: Jujupy ModelClient object
    :return: Dict of list-models command
    """
    try:
        raw = client.get_juju_output('list-models', '--format', 'json',
                                     include_e=False)
    except subprocess.CalledProcessError as e:
        log.error('Failed to list current models due to error: {}'.format(e))
        raise e
    return json.loads(raw)


def is_subordinate(app_data):
    return ('unit' not in app_data) and ('subordinate-to' in app_data)


def application_machines_from_app_info(app_data):
    """Get all the machines used to host the given application from the
       application info in status.

    :param app_data: application info from status
    """
    machines = [unit_data['machine'] for unit_data in
                app_data['units'].values()]
    return machines


def subordinate_machines_from_app_info(app_data, apps):
    """Get the subordinate machines from a given application from the
       application info in status.

    :param app_data: application info from status
    """
    machines = []
    for sub_name in app_data['subordinate-to']:
        for app_name, prim_app_data in apps.items():
            if sub_name == app_name:
                machines.extend(application_machines_from_app_info(
                    prim_app_data))
    return machines


def align_machine_profiles(machine_profiles):
    """Align machine profiles will create a dict from a list of machine
       ensuring that the machines are unique to each charm profile name.

    :param machine_profiles: is a list of machine profiles tuple
    """
    result = {}
    for items in machine_profiles:
        charm_profile = items[0]
        if charm_profile in result:
            # drop duplicates using set difference
            a = set(result[charm_profile])
            b = set(items[1])
            result[charm_profile].extend(b.difference(a))
        else:
            result[charm_profile] = list(items[1])
    return result
