"""Testing helpers and base classes for better isolation."""

from contextlib import contextmanager
import datetime
import logging
import os
import io
try:
    from StringIO import StringIO
except ImportError:
    from io import StringIO
import subprocess
import sys
from tempfile import NamedTemporaryFile
import unittest

try:
    from mock import patch
except ImportError:
    from unittest.mock import patch
import yaml

import utility


@contextmanager
def stdout_guard():
    if isinstance(sys.stdout, io.TextIOWrapper):
        stdout = io.StringIO()
    else:
        stdout = io.BytesIO()
    with patch('sys.stdout', stdout):
        yield
    if stdout.getvalue() != '':
        raise AssertionError(
            'Value written to stdout: {}'.format(stdout.getvalue()))


def use_context(test_case, context):
    result = context.__enter__()
    test_case.addCleanup(context.__exit__, None, None, None)
    return result


class TestCase(unittest.TestCase):
    """TestCase provides a better isolated version of unittest.TestCase."""

    log_level = logging.INFO
    test_environ = {}

    def setUp(self):
        super(TestCase, self).setUp()

        def _must_not_Popen(*args, **kwargs):
            """Tests may patch Popen but should never call it."""
            self.fail("subprocess.Popen(*{!r}, **{!r}) called".format(
                args, kwargs))

        self.addCleanup(setattr, subprocess, "Popen", subprocess.Popen)
        subprocess.Popen = _must_not_Popen

        self.addCleanup(setattr, os, "environ", os.environ)
        os.environ = dict(self.test_environ)

        setup_test_logging(self, self.log_level)

    def assertIsTrue(self, expr, msg=None):
        """Assert that expr is the True object."""
        self.assertIs(expr, True, msg)

    def assertIsFalse(self, expr, msg=None):
        """Assert that expr is the False object."""
        self.assertIs(expr, False, msg)

    def addContext(self, context):
        """Enter context manager for the remainder of the test, then leave.

        This can be used in place of a with block in setUp, which must return
        and may not yield. Note that exceptions will not be passed in when
        calling __exit__."""
        self.addCleanup(context.__exit__, None, None, None)
        return context.__enter__()


if getattr(TestCase, 'assertItemsEqual', None) is None:
    TestCase.assertItemsEqual = TestCase.assertCountEqual


class FakeHomeTestCase(TestCase):
    """FakeHomeTestCase creates an isolated home dir for Juju to use."""

    def setUp(self):
        super(FakeHomeTestCase, self).setUp()
        self.home_dir = use_context(self, utility.temp_dir())
        os.environ['HOME'] = self.home_dir
        os.environ['PATH'] = os.path.join(self.home_dir, '.local', 'bin')
        self.juju_home = os.path.join(self.home_dir, '.juju')
        os.mkdir(self.juju_home)
        self.set_public_clouds(get_default_public_clouds())

    def set_public_clouds(self, data_dict):
        """Set the data in the public-clouds.yaml file.

        :param data_dict: A dictionary of data, which is used to overwrite
            the data in public-clouds.yaml, or None, in which case the file
            is removed."""
        dest_file = os.path.join(self.juju_home, 'public-clouds.yaml')
        if data_dict is None:
            with utility.skip_on_missing_file():
                os.remove(dest_file)
        else:
            with open(dest_file, 'w') as file:
                yaml.safe_dump(data_dict, file)


def setup_test_logging(testcase, level=None):
    log = logging.getLogger()
    testcase.addCleanup(setattr, log, 'handlers', log.handlers)
    log.handlers = []
    testcase.log_stream = StringIO()
    handler = logging.StreamHandler(testcase.log_stream)
    handler.setFormatter(logging.Formatter("%(levelname)s %(message)s"))
    log.addHandler(handler)
    if level is not None:
        testcase.addCleanup(log.setLevel, log.level)
        log.setLevel(level)


# suppress nosetests
setup_test_logging.__test__ = False


@contextmanager
def parse_error(test_case):
    if isinstance(sys.stdout, io.TextIOWrapper):
        stderr = io.StringIO()
    else:
        stderr = io.BytesIO()
    with test_case.assertRaises(SystemExit):
        with patch('sys.stderr', stderr):
            yield stderr


@contextmanager
def temp_os_env(key, value):
    """Set the environment key to value for the context, then restore it."""
    org_value = os.environ.get(key, '')
    os.environ[key] = value
    try:
        yield
    finally:
        os.environ[key] = org_value


def assert_juju_call(test_case, mock_method, client, expected_args,
                     call_index=None):
    """Check a mock's positional arguments.

    :param test_case: The test case currently being run.
    :param mock_mothod: The mock object to be checked.
    :param client: Ignored.
    :param expected_args: The expected positional arguments for the call.
    :param call_index: Index of the call to check, if None checks first call
    and checks for only one call."""
    if call_index is None:
        test_case.assertEqual(len(mock_method.mock_calls), 1)
        call_index = 0
    empty, args, kwargs = mock_method.mock_calls[call_index]
    test_case.assertEqual(args, (expected_args,))


class FakePopen(object):
    """Create an artifical version of the Popen class."""

    def __init__(self, out, err, returncode):
        self._out = out if out is None else out.encode('ascii')
        self._err = err if err is None else err.encode('ascii')
        self._code = returncode

    def communicate(self):
        self.returncode = self._code
        return self._out, self._err

    def poll(self):
        return self._code


@contextmanager
def observable_temp_file():
    """Get a name which is used to create temporary files in the context."""
    temporary_file = NamedTemporaryFile(delete=False)
    try:
        with temporary_file as temp_file:

            @contextmanager
            def nt():
                # This is used to prevent NamedTemporaryFile.close from being
                # called.
                yield temporary_file

            with patch('jujupy.utility.NamedTemporaryFile',
                       return_value=nt()):
                yield temp_file
    finally:
        # File may have already been deleted, e.g. by temp_yaml_file.
        with utility.skip_on_missing_file():
            os.unlink(temporary_file.name)


@contextmanager
def client_past_deadline(client):
    """Create a client patched to be past its deadline."""
    soft_deadline = datetime.datetime(2015, 1, 2, 3, 4, 6)
    now = soft_deadline + datetime.timedelta(seconds=1)
    old_soft_deadline = client._backend.soft_deadline
    client._backend.soft_deadline = soft_deadline
    try:
        with patch.object(client._backend, '_now', return_value=now,
                          autospec=True):
            yield client
    finally:
        client._backend.soft_deadline = old_soft_deadline


def get_default_public_clouds():
    """The dict used to fill public-clouds.yaml by FakeHomeTestCase."""
    return {
        'clouds': {
            'foo': {
                'type': 'foo',
                'auth-types': ['access-key'],
                'regions': {
                    # This is the fake juju endpoint:
                    'bar': {'endpoint': 'bar.foo.example.com'},
                    'fee': {'endpoint': 'fee.foo.example.com'},
                    'fi': {'endpoint': 'fi.foo.example.com'},
                    'foe': {'endpoint': 'foe.foo.example.com'},
                    'fum': {'endpoint': 'fum.foo.example.com'},
                    }
                },
            'qux': {
                'type': 'fake',
                'auth-types': ['access-key'],
                'regions': {
                    'north': {'endpoint': 'north.qux.example.com'},
                    'south': {'endpoint': 'south.qux.example.com'},
                    }
                },
            }
        }
