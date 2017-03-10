"""Tests for assess_endpoint_bindings module."""

import logging
from mock import (
    call,
    Mock,
    patch,
    )
import StringIO

from assess_endpoint_bindings import (
    bootstrap_and_test,
    create_test_charms,
    ensure_spaces,
    parse_args,
    machine_spaces_for_bundle,
    main,
)
from jujupy.client import JujuData
from tests import (
    parse_error,
    TestCase,
)
from test_deploy_stack import FakeBootstrapManager


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_no_upload_tools(self):
        with parse_error(self) as fake_stderr:
            parse_args(["an-env", "/bin/juju", "--upload-tools"])
        self.assertIn(
            "error: giving --upload-tools meaningless on 2.0 only test",
            fake_stderr.getvalue())

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn("endpoint bindings", fake_stdout.getvalue())


class TestEnsureSpaces(TestCase):

    default_space = {
        "name": "Default space",
        "id": 0,
        "resource_uri": "/MAAS/api/2.0/spaces/0/",
        "subnets": [
            {
                "space": "Default space",
                "id": 2,
            },
        ],
    }
    alpha_space = {
        "name": "alpha-space",
        "id": 60,
        "resource_uri": "/MAAS/api/2.0/spaces/60/",
        "subnets": [],
    }
    beta_space = {
        "name": "beta-space",
        "id": 61,
        "resource_uri": "/MAAS/api/2.0/spaces/61/",
        "subnets": [],
    }

    def test_all_existing(self):
        manager = Mock(spec=["spaces"])
        manager.spaces.return_value = [self.default_space, self.alpha_space]
        spaces = ensure_spaces(manager, ["alpha-space"])
        self.assertEqual(spaces, [self.alpha_space])
        manager.spaces.assert_called_once_with()
        self.assertEqual(
            "INFO Have spaces: Default space, alpha-space\n",
            self.log_stream.getvalue())

    def test_some_existing(self):
        manager = Mock(spec=["create_space", "spaces"])
        manager.create_space.return_value = self.alpha_space
        manager.spaces.return_value = [self.default_space, self.beta_space]
        spaces = ensure_spaces(manager, ["alpha-space", "beta-space"])
        self.assertEqual(spaces, [self.alpha_space, self.beta_space])
        manager.spaces.assert_called_once_with()
        manager.create_space.assert_called_once_with("alpha-space")
        self.assertRegexpMatches(
            self.log_stream.getvalue(),
            r"^INFO Have spaces: Default space, beta-space\n"
            r"INFO Created space: \{.*\}\n$")


class TestMachineSpacesForBundle(TestCase):

    def test_no_bindings(self):
        bundle_without_bindings = {
            "services": {
                "ubuntu": {
                    "charm": "cs:ubuntu",
                    "num_units": 3,
                },
            },
        }
        machines = machine_spaces_for_bundle(bundle_without_bindings)
        self.assertEqual(machines, [frozenset(), frozenset(), frozenset()])

    def test_single_binding(self):
        bundle_without_bindings = {
            "services": {
                "anapp": {
                    "charm": "./anapp",
                    "series": "xenial",
                    "num_units": 1,
                    "bindings": {
                        "website": "space-public",
                    },
                },
            },
        }
        machines = machine_spaces_for_bundle(bundle_without_bindings)
        self.assertEqual(machines, [frozenset(["space-public"])])

    def test_multiple_bindings(self):
        bundle_without_bindings = {
            "services": {
                "anapp": {
                    "charm": "./anapp",
                    "series": "xenial",
                    "num_units": 1,
                    "bindings": {
                        "website": "space-public",
                        "data": "space-data",
                        "monitoring": "space-ctl",
                    },
                },
                "adb": {
                    "charm": "./adb",
                    "series": "xenial",
                    "num_units": 2,
                    "bindings": {
                        "data": "space-data",
                        "monitoring": "space-ctl",
                    },
                },
            },
        }
        machines = machine_spaces_for_bundle(bundle_without_bindings)
        app_spaces = frozenset(["space-data", "space-ctl", "space-public"])
        db_spaces = frozenset(["space-data", "space-ctl"])
        self.assertEqual(machines, [app_spaces, db_spaces, db_spaces])


class AssessEndpointBindings(TestCase):

    def test_create_test_charms(self):
        bundle, charms = create_test_charms()
        self.assertEqual(
            ['0', '1', '2'],
            sorted(bundle['machines'].keys()))
        self.assertEqual(
            ['datastore', 'frontend', 'monitor'],
            sorted(bundle['services'].keys()))
        self.assertEqual('datastore', charms[0].metadata['name'])
        self.assertEqual('frontend', charms[1].metadata['name'])

    def test_bootstrap_and_test(self):
        juju_data = JujuData(
            'foo', {'type': 'bar', 'region': 'region'}, juju_home='baz')
        client = Mock(
            spec=['bootstrap', 'kill_controller', 'deploy',
                  'wait_for_started', 'wait_for_workloads'],
            env=juju_data)
        bootstrap_manager = FakeBootstrapManager(client)
        bootstrap_and_test(bootstrap_manager, 'bundle_path', None)
        self.assertEqual(
            [call('bundle_path'),
             call('./xenial/frontend',
                  bind='endpoint-bindings-data', alias='adminsite')],
            client.deploy.mock_calls)
        self.assertEqual(2, client.wait_for_started.call_count)
        self.assertEqual(2, client.wait_for_workloads.call_count)


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = Mock(spec=["config"])
        client = Mock(spec=["env", "is_jes_enabled"])
        client.env = env
        with patch("assess_endpoint_bindings.configure_logging",
                   autospec=True) as mock_cl:
            with patch(
                    "assess_endpoint_bindings.maas_account_from_boot_config",
                    autospec=True) as mock_ma:
                with patch("deploy_stack.client_from_config",
                           return_value=client) as mock_c:
                    with patch(
                        "assess_endpoint_bindings.assess_endpoint_bindings",
                            autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with(
            "an-env", "/bin/juju", debug=False, soft_deadline=None)
        self.assertEqual(mock_ma.call_count, 1)
        self.assertEqual(mock_assess.call_count, 1)
