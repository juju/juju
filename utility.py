from contextlib import contextmanager
import errno
import os
from shutil import rmtree
import socket
import sys
from time import sleep
from tempfile import mkdtemp

from jujupy import until_timeout


@contextmanager
def scoped_environ():
    old_environ = dict(os.environ)
    try:
        yield
    finally:
        os.environ.clear()
        os.environ.update(old_environ)


class PortTimeoutError(Exception):
    pass


def wait_for_port(host, port, closed=False, timeout=30):
    for remaining in until_timeout(timeout):
        conn = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        conn.settimeout(max(remaining, 5))
        try:
            conn.connect((host, port))
        except socket.timeout:
            if closed:
                return
        except socket.error as e:
            if e.errno not in (errno.ECONNREFUSED, errno.ETIMEDOUT):
                raise
            if closed:
                return
        except Exception as e:
            print_now('Unexpected %r: %s' % (type(e), e))
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
def temp_dir(parent=None):
    directory = mkdtemp(dir=parent)
    try:
        yield directory
    finally:
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
    :param revision_build: The revision_build to searh cofr. Note that
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
