from contextlib import contextmanager
from datetime import (
    datetime,
    timedelta,
    )
import errno
import json
import logging
import os
import re
from shutil import rmtree
import subprocess
import socket
import sys
from time import (
    sleep,
    time,
    )
from tempfile import (
    mkdtemp,
    NamedTemporaryFile,
    )
import warnings
# Export shell quoting function which has moved in newer python versions
try:
    from shlex import quote
except ImportError:
    from pipes import quote
import yaml

quote


# Equivalent of socket.EAI_NODATA when using windows sockets
# <https://msdn.microsoft.com/ms740668#WSANO_DATA>
WSANO_DATA = 11004


@contextmanager
def noop_context():
    """A context manager that does nothing."""
    yield


@contextmanager
def scoped_environ(new_environ=None):
    """Save the current environment and restore it when the context is exited.

    :param new_environ: If provided and not None, the key/value pairs of the
    iterable are used to create a new environment in the context."""
    old_environ = dict(os.environ)
    try:
        if new_environ is not None:
            os.environ.clear()
            os.environ.update(new_environ)
        yield
    finally:
        os.environ.clear()
        os.environ.update(old_environ)


class PortTimeoutError(Exception):
    pass


class LoggedException(BaseException):
    """Raised in place of an exception that has already been logged.

    This is a wrapper to avoid double-printing real Exceptions while still
    unwinding the stack appropriately.
    """
    def __init__(self, exception):
        self.exception = exception


class until_timeout:

    """Yields remaining number of seconds.  Stops when timeout is reached.

    :ivar timeout: Number of seconds to wait.
    """
    def __init__(self, timeout, start=None):
        self.timeout = timeout
        if start is None:
            start = self.now()
        self.start = start

    def __iter__(self):
        return self

    @staticmethod
    def now():
        return datetime.now()

    def __next__(self):
        self.next()

    def next(self):
        elapsed = self.now() - self.start
        remaining = self.timeout - elapsed.total_seconds()
        if remaining <= 0:
            raise StopIteration
        return remaining


class JujuAssertionError(AssertionError):
    """Exception for juju assertion failures."""


class JujuResourceTimeout(Exception):
    """A timeout exception for a resource not being downloaded into a unit."""


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
            warnings.warn('Not a directory {}'.format(maybe_dir))
        if e.errno == errno.EEXIST:
            warnings.warn('Directory {} already exists'.format(maybe_dir))
    else:
        if contents and contents != ["empty"]:
            warnings.warn(
                'Directory {!r} has existing contents.'.format(maybe_dir))
    return maybe_dir


def pause(seconds):
    print_now('Sleeping for {:d} seconds.'.format(seconds))
    sleep(seconds)


def is_ipv6_address(address):
    """Returns True if address is IPv6 rather than IPv4 or a host name.

    Incorrectly returns False for IPv6 addresses on windows due to lack of
    support for socket.inet_pton there.
    """
    try:
        socket.inet_pton(socket.AF_INET6, address)
    except (AttributeError, socket.error):
        # IPv4 or hostname
        return False
    return True


def as_literal_address(address):
    """Returns address in form suitable for embedding in URL or similar.

    In practice, this just puts square brackets round IPv6 addresses which
    avoids conflict with port seperators and other uses of colons.
    """
    if is_ipv6_address(address):
        return address.join("[]")
    return address


def split_address_port(address_port):
    """Split an ipv4 or ipv6 address and port into a tuple.

    ipv6 addresses must be in the literal form with a port ([::12af]:80).
    ipv4 addresses may be without a port, which translates to None.
    """
    if ':' not in address_port:
        # This is correct for ipv4.
        return address_port, None
    address, port = address_port.rsplit(':', 1)
    address = address.strip('[]')
    return address, port


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
            print_now('Unexpected {!r}: {}'.format((type(e), e)))
            raise
        else:
            conn.close()
            if not closed:
                return
            sleep(1)
    raise PortTimeoutError('Timed out waiting for port.')


def print_now(string):
    print(string)
    sys.stdout.flush()


@contextmanager
def temp_dir(parent=None, keep=False):
    directory = mkdtemp(dir=parent)
    try:
        yield directory
    finally:
        if not keep:
            rmtree(directory)


def get_revision_build(build_info):
    for action in build_info['actions']:
        if 'parameters' in action:
            for parameter in action['parameters']:
                if parameter['name'] == 'revision_build':
                    return parameter['value']


def builds_for_revision(job, revision_build, jenkins):
    """Return the build_info data for the given job and revision_build.

    Only successful builds are included.

    :param job: The name of the job.
    :param revision_build: The revision_build to searh for. Note that
        this parameter is a string.
    :parameter  jenkins: A Jenkins instance.
    """
    job_info = jenkins.get_job_info(job)
    result = []
    for build in job_info['builds']:
        build_info = jenkins.get_build_info(job, build['number'])
        if (get_revision_build(build_info) == revision_build and
                build_info['result'] == 'SUCCESS'):
            result.append(build_info)
    return result


def check_free_disk_space(path, required, purpose):
    df_result = subprocess.check_output(["df", "-k", path])
    df_result = df_result.split('\n')[1]
    df_result = re.split(' +', df_result)
    available = int(df_result[3])
    if available < required:
        message = (
            "Warning: Probably not enough disk space available for\n"
            "%(purpose)s in directory %(path)s,\n"
            "mount point %(mount)s\n"
            "required: %(required)skB, available: %(available)skB."
            )
        print(message % {
            'path': path, 'mount': df_result[5], 'required': required,
            'available': available, 'purpose': purpose
            })


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
    except:
        return 'unknown_test'

def _generate_default_clean_dir(temp_env_name):
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
            raise('Failed to create logging directory: {} ' +
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


def add_basic_testing_arguments(parser, using_jes=False, deadline=True,
                                env=True):
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
                        ' this will generate an enviroment name using the '
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
    parser.add_argument('--agent-url', action='store', default=None,
                        help='URL for retrieving agent binaries.')
    parser.add_argument('--agent-stream', action='store', default=None,
                        help='Stream for retrieving agent binaries.')
    parser.add_argument('--series', action='store', default=None,
                        help='Name of the Ubuntu series to use.')
    if not using_jes:
        parser.add_argument('--upload-tools', action='store_true',
                            help='upload local version of tools to bootstrap.')
    parser.add_argument('--bootstrap-host',
                        help='The host to use for bootstrap.')
    parser.add_argument('--machine', help='A machine to add or when used with '
                        'KVM based MaaS, a KVM image to start.',
                        action='append', default=[])
    parser.add_argument('--keep-env', action='store_true',
                        help='Keep the Juju environment after the test'
                        ' completes.')
    if deadline:
        parser.add_argument('--timeout', dest='deadline', type=_to_deadline,
                            help="The script timeout, in seconds.")
    return parser


# suppress nosetests
add_basic_testing_arguments.__test__ = False


def configure_logging(log_level):
    logging.basicConfig(
        level=log_level, format='%(asctime)s %(levelname)s %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S')


@contextmanager
def skip_on_missing_file():
    """Skip to the end of block if a missing file exception is raised."""
    try:
        yield
    except (IOError, OSError) as e:
        if e.errno != errno.ENOENT:
            raise


def ensure_dir(path):
    try:
        os.mkdir(path)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise


def ensure_deleted(path):
    with skip_on_missing_file():
        os.unlink(path)


@contextmanager
def temp_yaml_file(yaml_dict, encoding="utf-8"):
    temp_file = NamedTemporaryFile(suffix='.yaml', delete=False)
    try:
        with temp_file:
            yaml.safe_dump(yaml_dict, temp_file, encoding=encoding)
        yield temp_file.name
    finally:
        os.unlink(temp_file.name)


def get_candidates_path(root_dir):
    return os.path.join(root_dir, 'candidate')


def get_timeout_path():
    import timeout
    return os.path.abspath(timeout.__file__)


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


def unqualified_model_name(model_name):
    """Return the model name with the owner qualifier stripped if present."""
    return model_name.split('/', 1)[-1]


def qualified_model_name(model_name, owner_name):
    """Return the model name qualified with the given owner name."""
    if model_name == '' or owner_name == '':
        raise ValueError(
            'Neither model_name nor owner_name can be blank strings')

    parts = model_name.split('/', 1)
    if len(parts) == 2 and parts[0] != owner_name:
        raise ValueError(
            'qualified model name {} with owner not matching {}'.format(
                model_name, owner_name))
    return '{}/{}'.format(owner_name, parts[-1])


def get_unit_ipaddress(client, unit_name):
    status = client.get_status()
    return status.get_unit(unit_name)['public-address']


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
