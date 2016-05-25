"""Tests for assess_multi_series_charms module."""

import logging
from mock import (
    call,
    Mock,
    patch,
)
import os
import StringIO
import subprocess

from assess_multi_series_charms import (
    assert_deploy,
    assess_multi_series_charms,
    parse_args,
    main,
    Test,
)
from tests import (
    parse_error,
    TestCase,
)
from utility import (
    JujuAssertionError,
    temp_dir,
)


class TestParseArgs(TestCase):
    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):
    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_multi_series_charms.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_multi_series_charms.BootstrapManager."
                       "booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_multi_series_charms."
                                   "assess_multi_series_charms",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_assess_multi_series_charms(self):
        mock_client = Mock(
            spec=["deploy", "get_juju_output", "wait_for_started",
                  "is_juju1x"])
        mock_client.version = '2.0.0'
        mock_client.is_juju1x.return_value = False
        mock_client.get_juju_output.return_value = "Codename:	trusty"
        mock_client.deploy.side_effect = [
            subprocess.CalledProcessError(None, None),
            None,
            None,
            None,
            None
        ]
        with temp_dir() as charm_dir:
            with patch('assess_multi_series_charms.temp_dir',
                       autospec=True) as td_mock:
                td_mock.return_value.__enter__.return_value = charm_dir
                with patch('assess_multi_series_charms.check_series',
                           autospec=True) as cs_mock:
                    assess_multi_series_charms(mock_client)
        self.assertEqual(mock_client.wait_for_started.call_count, 4)
        charm = os.path.join(charm_dir, 'trusty', 'dummy')
        calls = [
            call(charm=charm, force=False, repository=charm_dir,
                 series='precise', service='test0'),
            call(charm=charm, force=False, repository=charm_dir,
                 series=None, service='test1'),
            call(charm=charm, force=False, repository=charm_dir,
                 series='trusty', service='test2'),
            call(charm=charm, force=False, repository=charm_dir,
                 series='xenial', service='test3'),
            call(charm=charm, force=True, repository=charm_dir,
                 series='precise', service='test4')
        ]
        self.assertEqual(mock_client.deploy.mock_calls, calls)
        td_mock.assert_called_once_with()
        cs_calls = [
            call(mock_client, machine='0', series=None),
            call(mock_client, machine='1', series='trusty'),
            call(mock_client, machine='2', series='xenial'),
            call(mock_client, machine='3', series='precise')]
        self.assertEqual(cs_mock.mock_calls, cs_calls)

    def test_assess_multi_series_charms_juju1x(self):
        mock_client = Mock(
            spec=[
                "deploy", "get_juju_output", "wait_for_started", "is_juju1x"])
        mock_client.version = '1.25.5'
        mock_client.is_juju1x.return_value = True
        mock_client.get_juju_output.return_value = "Codename:	trusty"
        mock_client.deploy.return_value = None
        with temp_dir() as charm_dir:
            with patch('assess_multi_series_charms.temp_dir',
                       autospec=True) as td_mock:
                td_mock.return_value.__enter__.return_value = charm_dir
                with patch('assess_multi_series_charms.check_series',
                           autospec=True) as cs_mock:
                    assess_multi_series_charms(mock_client)
        self.assertEqual(mock_client.wait_for_started.call_count, 2)
        charm = os.path.join('local:trusty', 'dummy')
        calls = [
            call(charm=charm, force=False, repository=charm_dir,
                 series=None, service='test1'),
            call(charm=charm, force=False, repository=charm_dir,
                 series='trusty', service='test2')
        ]
        self.assertEqual(mock_client.deploy.mock_calls, calls)
        td_mock.assert_called_once_with()
        cs_calls = [
            call(mock_client, machine='0', series=None),
            call(mock_client, machine='1', series='trusty')]
        self.assertEqual(cs_mock.mock_calls, cs_calls)

    def test_assert_deploy(self):
        test = Test(series='trusty', service='test1', force=False,
                    success=True, machine='0', juju1x_supported=False)
        mock_client = Mock(
            spec=["deploy", "get_juju_output", "wait_for_started"])
        assert_deploy(mock_client, test, '/tmp/foo')
        mock_client.deploy.assert_called_once_with(
            charm='/tmp/foo', force=False, repository=None, series='trusty',
            service='test1')

    def test_assert_deploy_success_false(self):
        test = Test(series='trusty', service='test1', force=False,
                    success=False, machine='0',  juju1x_supported=False)
        mock_client = Mock(
            spec=["deploy", "get_juju_output", "wait_for_started"])
        mock_client.deploy.side_effect = subprocess.CalledProcessError(
            None, None)
        assert_deploy(mock_client, test, '/tmp/foo')
        mock_client.deploy.assert_called_once_with(
            charm='/tmp/foo', force=False, repository=None, series='trusty',
            service='test1')

    def test_assert_deploy_success_false_raises_exception(self):
        test = Test(series='trusty', service='test1', force=False,
                    success=False, machine='0',  juju1x_supported=False)
        mock_client = Mock(
            spec=["deploy", "get_juju_output", "wait_for_started"])
        with self.assertRaisesRegexp(
                JujuAssertionError, 'Assert deploy failed for'):
            assert_deploy(mock_client, test, '/tmp/foo')
        mock_client.deploy.assert_called_once_with(
            charm='/tmp/foo', force=False, repository=None, series='trusty',
            service='test1')
