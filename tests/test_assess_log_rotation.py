from argparse import Namespace
from contextlib import contextmanager

from mock import (
    Mock,
    patch,
)

from assess_log_rotation import (
    check_expected_backup,
    check_for_extra_backup,
    check_log0,
    LogRotateError,
    make_client_from_args,
    parse_args,
    test_debug_log,
    test_machine_rotation,
)
from jujupy import (
    EnvJujuClient,
    JujuData,
    _temp_env as temp_env,
    yaml_loads,
    )
from tests import TestCase
from tests.test_jujupy import FakeJujuClient

good_yaml = \
    """
results:
  result-map:
    log0:
      name: /var/log/juju/unit-fill-logs-0.log
      size: "25"
    log1:
      name: /var/log/juju/unit-fill-logs-0-2015-05-21T09-57-03.123.log
      size: "299"
    log1:
      name: /var/log/juju/unit-fill-logs-0-2015-05-22T12-57-03.123.log
      size: "300"
status: completed
timing:
  completed: 2015-05-21 09:57:03 -0400 EDT
  enqueued: 2015-05-21 09:56:59 -0400 EDT
  started: 2015-05-21 09:57:02 -0400 EDT
"""

good_obj = yaml_loads(good_yaml)

big_yaml = \
    """
results:
  result-map:
    log0:
      name: /var/log/juju/unit-fill-logs-0.log
      size: "400"
    log1:
      name: /var/log/juju/unit-fill-logs-0-2015-05-21T09-57-03.123.log
      size: "400"
    log2:
      name: /var/log/juju/unit-fill-logs-0-not-a-valid-timestamp.log
      size: "299"
    log3:
      name: something-just-plain-bad.log
      size: "299"
status: completed
timing:
  completed: 2015-05-21 09:57:03 -0400 EDT
  enqueued: 2015-05-21 09:56:59 -0400 EDT
  started: 2015-05-21 09:57:02 -0400 EDT
"""

big_obj = yaml_loads(big_yaml)

no_files_yaml = \
    """
results:
  result-map:
status: completed
timing:
  completed: 2015-05-21 09:57:03 -0400 EDT
  enqueued: 2015-05-21 09:56:59 -0400 EDT
  started: 2015-05-21 09:57:02 -0400 EDT
"""

no_files_obj = yaml_loads(no_files_yaml)


class TestCheckForExtraBackup(TestCase):

    def test_not_found(self):
        try:
            # log2 should not be found, and thus no exception.
            check_for_extra_backup("log2", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_find_extra(self):
        with self.assertRaises(LogRotateError):
            # log1 should be found, and thus cause an exception.
            check_for_extra_backup("log1", good_obj)


class TestCheckBackup(TestCase):

    def test_exists(self):
        try:
            # log1 should be found, and thus no exception.
            check_expected_backup("log1", "unit-fill-logs-0", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_not_found(self):
        with self.assertRaises(LogRotateError):
            # log2 should not be found, and thus cause an exception.
            check_expected_backup("log2", "unit-fill-logs-0", good_obj)

    def test_too_big(self):
        with self.assertRaises(LogRotateError):
            # log1 is too big, and thus should cause an exception.
            check_expected_backup("log1", "unit-fill-logs-0", big_obj)

    def test_bad_timestamp(self):
        with self.assertRaises(LogRotateError):
            # log2 has an invalid timestamp, and thus should cause an
            # exception.
            check_expected_backup("log2", "unit-fill-logs-0", big_obj)

    def test_bad_name(self):
        with self.assertRaises(LogRotateError):
            # log3 has a completely invalid name, and thus should cause an
            # exception.
            check_expected_backup("log3", "unit-fill-logs-0", big_obj)


class TestCheckLog0(TestCase):

    def test_exists(self):
        try:
            # log0 should be found, and thus no exception.
            check_log0("/var/log/juju/unit-fill-logs-0.log", good_obj)
        except Exception as e:
            self.fail("unexpected exception: %s" % e.msg)

    def test_not_found(self):
        with self.assertRaises(AttributeError):
            # There's no value under result-map, which causes the yaml parser
            # to consider it None, and thus it'll cause an AttributeError
            check_log0("/var/log/juju/unit-fill-logs-0.log", no_files_obj)

    def test_too_big(self):
        with self.assertRaises(LogRotateError):
            # log0 is too big, and thus should cause an exception.
            check_log0(
                "/var/log/juju/unit-fill-logs-0.log", big_obj)


class TestTestDebugLog(TestCase):

    def test_happy_log(self):
        client = Mock()
        client.get_juju_output.return_value = '\n'*100
        # Ensure that no exception is raised
        test_debug_log(client, timeout=120)
        client.get_juju_output.assert_called_once_with(
            "debug-log", "--lines=100", "--limit=100", timeout=120)

    def test_unhappy_log(self):
        client = Mock()
        client.get_juju_output.return_value = ''
        # Ensure that no exception is raised
        with self.assertRaises(LogRotateError):
            test_debug_log(client)
        client.get_juju_output.assert_called_once_with(
            "debug-log", "--lines=100", "--limit=100", timeout=180)


class TestMachineRoation(TestCase):

    def test_respects_machine_id_0(self):
        client = FakeJujuClient(jes_enabled=True)
        client.bootstrap()
        client.deploy('fill-logs')
        with patch('assess_log_rotation.test_rotation') as tr_mock:
            test_machine_rotation(client)
        tr_mock.assert_called_once_with(
            client, '/var/log/juju/machine-0.log', 'machine-0', 'fill-machine',
            'machine-size', 'megs=300', 'machine=0')

    def test_respects_machine_id_1(self):
        client = FakeJujuClient(jes_enabled=False)
        client.bootstrap()
        client.deploy('fill-logs')
        with patch('assess_log_rotation.test_rotation') as tr_mock:
            test_machine_rotation(client)
        tr_mock.assert_called_once_with(
            client, '/var/log/juju/machine-1.log', 'machine-1',
            'fill-machine', 'machine-size', 'megs=300', 'machine=1')


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['b', 'c/juju', 'd', 'e', 'machine'])
        self.assertEqual(args, Namespace(
            agent='machine', env='b', juju_bin='c/juju', logs='d',
            temp_env_name='e', debug=False, agent_stream=None, agent_url=None,
            bootstrap_host=None, machine=[], keep_env=False,
            region=None, series=None, upload_tools=False, verbose=20))

    def test_parse_args_unit(self):
        args = parse_args(['b', 'c/juju', 'd', 'e', 'unit'])
        self.assertEqual('unit', args.agent)


class TestMakeClientFromArgs(TestCase):

    @contextmanager
    def make_client_cxt(self):
        with temp_env({'environments': {'foo': {}}}):
            with patch('subprocess.check_output', return_value=''):
                with patch('jujupy.EnvJujuClient.get_jes_command',
                           autospec=True, return_value='controller'):
                    with patch('jujupy.EnvJujuClient.juju',
                               autospec=True, return_value=''):
                        with patch('assess_log_rotation.tear_down',
                                   autospec=True, return_value='') as td_func:
                            with patch.object(JujuData, 'load_yaml'):
                                yield td_func

    def test_defaults(self):
        with self.make_client_cxt() as td_func:
            client = make_client_from_args(Namespace(
                juju_bin='', debug=False, env='foo', temp_env_name='bar',
                agent_url=None, agent_stream=None, series=None, region=None,
                bootstrap_host=None, machine=[]
                ))
        self.assertIsInstance(client, EnvJujuClient)
        self.assertIn('/jes-homes/bar', client.env.juju_home)
        td_func.assert_called_once_with(client, True)
