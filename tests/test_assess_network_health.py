import yaml
import StringIO
import logging
from datetime import (
    datetime,
    timedelta,
    )
from textwrap import dedent


from mock import (
    call,
    patch,
    Mock,
    )
from tests import (
    TestCase,
    parse_error
    )
from jujupy import (
    fake_juju_client,
    Status
)
from assess_network_health import (
    main,
    setup_testing_environment,
    setup_dummy_deployment,
    setup_bundle_deployment,
    get_juju_status,
    juju_controller_visibility,
    internet_connection,
    neighbor_visibility,
    ensure_exposed,
    setup_expose_test,
    parse_expose_results,
    parse_final_results,
    parse_targets,
    parse_args,
    to_json,
    ping_units,
    ConnectionError
    )

apps = {'foo':
        {'charm-name': 'foo',
         'exposed': True,
         'units': {
            'foo/0': {'machine': '0',
                      'public-address': '1.1.1.1',
                      'subordinates': {
                        'bar/0': {
                            'public-address': '1.1.1.1'}}},
            'foo/1': {'machine': '1',
                      'public-address': '1.1.1.2',
                      'subordinates': {
                        'bar/1': {
                            'public-address': '1.1.1.2'}}}}},
        'bar':
        {'charm-name': 'bar',
         'exposed': False}}


bundle_string = dedent("""\
services:
  foo:
    charm: local:trusty/foo
    num_units: 1
    expose: true
  bar:
    charm: local:trusty/bar
    num_units: 1
series: trusty
relations:
- - foo:baz
  - bar:baz
""")

status_value = dedent("""\
    machines:
      "0":
        agent-state: started
        ip-addresses:
        - 1.1.1.1
      "1":
        agent-state: started
        ip-addresses:
        - 1.1.1.2
    applications:
      ubuntu:
        units:
          ubuntu/0:
            subordinates:
              network-health/0:
                juju-status:
                  current: idle
                public-address: 1.1.1.1
            juju-status:
              current: idle
            public-address: 1.1.1.1
          ubuntu/1:
            subordinates:
              network-health/1:
                juju-status:
                  current: idle
                public-address: 1.1.1.2
            juju-status:
              current: idle
            public-address: 1.1.1.2
        application-status:
          current: unknown
          since: 01 Jan 2017 00:00:00-00:00
        exposed: true
      network-health:
        application-status:
          current: unknown
          since: 01 Jan 2017 00:00:00-00:00
        subordinate-to:
        - ubuntu
        relations:
          juju-info:
          - network-health
""")
status = Status(yaml.safe_load(status_value), status_value)

ping_result = dedent("""
results:
  results: '{u''ubuntu/0'': True, u''ubuntu/1'': True}'
status: completed
timing:
  completed: 2017-01-01 00:00:01 +0000 UTC
  enqueued: 2017-01-01 00:00:01 +0000 UTC
  started: 2017-01-01 00:00:01 +0000 UTC
""")

bundle_yaml = yaml.safe_load(bundle_string)
dummy_charm = 'dummy'
series = 'trusty'


class TestAssessNetworkHealth(TestCase):

    def test_setup_testing_environment(self):

        def setup_iteration(bundle, target_model, series):
            mock_client = Mock(spec=["juju", "wait_for_started",
                                     "wait_for_workloads", "deploy",
                                     "get_juju_output", "get_juju_status",
                                     "wait_for_subordinate_units",
                                     "get_status", "deploy_bundle"])
            status = Status(yaml.safe_load(status_value), status_value)
            mock_client.get_status.return_value = status
            mock_client.series = 'trusty'
            mock_client.version = '2.2'
            setup_testing_environment(mock_client, bundle, target_model,
                                      series)
            return mock_client

        client = setup_iteration(bundle=None, target_model=None, series=series)
        self.assertEqual(
            [call.deploy('ubuntu', num=2),
             call.juju('expose', ('ubuntu',)),
             call.deploy('~juju-qa/network-health'),
             call.wait_for_started(),
             call.wait_for_workloads(),
             call.get_status(),
             call.juju('add-relation', ('ubuntu', 'network-health')),
             call.wait_for_workloads(),
             call.wait_for_subordinate_units('ubuntu', 'network-health')],
            client.mock_calls)
        client = setup_iteration(bundle=bundle_string, target_model=None,
                                 series=series)
        self.assertEqual(
            [call.deploy_bundle('services:\n  foo:\n    '
                                'charm: local:trusty/foo\n    '
                                'num_units: 1\n    expose: true\n  '
                                'bar:\n    charm: local:trusty/bar\n    '
                                'num_units: 1\nseries: trusty\nrelations:\n'
                                '- - foo:baz\n  - bar:baz\n'),
             call.deploy('~juju-qa/network-health'),
             call.wait_for_started(),
             call.wait_for_workloads(),
             call.get_status(),
             call.juju('add-relation', ('ubuntu', 'network-health')),
             call.wait_for_workloads(),
             call.wait_for_subordinate_units('ubuntu', 'network-health')],
            client.mock_calls)

    def test_juju_controller_visibility(self):
        client = fake_juju_client()
        client.bootstrap()
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                with patch('subprocess.check_output',
                           return_value=0):
                    out = juju_controller_visibility(client)
        expected = {'1': {'1.1.1.2': True}, '0': {'1.1.1.1': True}}
        self.assertEqual(expected, out)

    def test_assess_network_health_with_existing_model(self):
        # Feature is not fully implimented, pass for now.
        pass

    def test_neighbor_visibility(self):
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        client._backend.set_action_result('network-health/0', 'ping',
                                          ping_result)
        client._backend.set_action_result('network-health/1', 'ping',
                                          ping_result)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                client.deploy('ubuntu', num=2)
                client.deploy('network-health')
                out = neighbor_visibility(client)
        expected = {'network-health/0': {'ubuntu': {u'ubuntu/0': True,
                                                    u'ubuntu/1': True}},
                    'network-health/1': {'ubuntu': {u'ubuntu/0': True,
                                                    u'ubuntu/1': True}}}
        self.assertEqual(expected, out)

    def test_internet_connection(self):
        client = fake_juju_client()
        client.bootstrap()
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                with patch('subprocess.check_output',
                           return_value=0):
                    out = internet_connection(client)
        expected = {'1': True, '0': True}
        self.assertEqual(expected, out)

    def test_ensure_exposed(self):
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        new_client = fake_juju_client()
        new_client.bootstrap()
        new_client._backend.set_action_result('network-health/0', 'ping',
                                              ping_result)
        new_client.deploy('ubuntu', num=2)
        new_client.deploy('network-health')
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                with patch('assess_network_health.setup_expose_test',
                           return_value=new_client):
                    out = ensure_exposed(client, series)
        expected = {'fail': (), 'pass': ('ubuntu',)}
        self.assertEqual(out, expected)

    def test_connect_to_existing_model(self):
        # Pass for now as this is not fully implimented
        pass

    def test_dummy_deployment(self):
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        setup_dummy_deployment(client, series)
        client.deploy.assert_called_once_with('ubuntu', num=2)

    def test_bundle_deployment(self):
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        setup_bundle_deployment(client, bundle_string)
        client.deploy_bundle.assert_called_once_with(bundle_string)

    def test_setup_expose_test(self):
        mock_client = Mock(spec=["juju", "wait_for_started",
                                 "wait_for_workloads", "deploy",
                                 "get_juju_output", "get_juju_status",
                                 "wait_for_subordinate_units",
                                 "get_status", "deploy_bundle", "add_model"
                                 ])
        mock_client.series = 'trusty'
        mock_client.version = '2.2'
        setup_expose_test(mock_client, series)
        self.assertEqual(
            [call.add_model('exposetest'),
             call.add_model().deploy('ubuntu'),
             call.add_model().deploy('~juju-qa/network-health'),
             call.add_model().wait_for_started(),
             call.add_model().wait_for_workloads(),
             call.add_model().juju('add-relation', ('ubuntu',
                                                    'network-health')),
             call.add_model().wait_for_subordinate_units('ubuntu',
                                                         'network-health')],
            mock_client.mock_calls)

    def test_get_juju_status(self):
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        expected = client.get_status().status
        result = get_juju_status(client)
        self.assertEqual(expected, result)

    def test_parse_expose_results(self):
        exposed = ['bar', 'baz']
        service_results = {'foo': "{'foo/0': 'True'}",
                           'bar': "{'bar/0': 'False'}",
                           'baz': "{'baz/0': 'True'}"}
        expected = {"fail": ('foo', 'bar'), "pass": ('baz',)}
        result = parse_expose_results(service_results, exposed)
        self.assertEqual(expected, result)

    def test_parse_final_results_with_fail(self):
        controller = {"0": {"1.1.1.1": False},
                      "1": {"1.1.1.2": True}}
        visible = {"bar/0": {"foo": {"foo/0": False, "foo/1": True}}}
        internet = {"0": False, "1": True}
        exposed = {"fail": ("foo"), "pass": ("bar", "baz")}
        with self.assertRaises(ConnectionError) as context:
            parse_final_results(controller, visible, internet, exposed)
        error_strings = ["Failed to contact controller from machine 0 "
                         "at address 1.1.1.1",
                         "Machine 0 failed internet connection.",
                         "NH-Unit bar/0 failed to contact unit(s): ['foo/0']",
                         "Application(s) foo failed expose test"]
        for line in error_strings:
            self.assertTrue(line in context.exception.message)

    def test_parse_final_results_without_fail(self):
        controller = {"0": {"1.1.1.1": True}}
        visible = {"bar/0": {"foo": {"foo/0": True, "foo/1": True}}}
        internet = {"0": True, "1": True}
        exposed = {"fail": (), "pass": ("foo", "bar", "baz")}
        parse_final_results(controller, visible, internet, exposed)

    def test_ping_units(self):
        client = fake_juju_client()
        client.bootstrap()
        client._backend.set_action_result('bar/0', 'ping', ping_result)
        targets = {'foo/0': '1.1.1.1'}
        out = ping_units(client, 'bar/0', targets)
        result = yaml.safe_load(ping_result)
        self.assertEqual(out, result['results']['results'])

    def test_to_json(self):
        expected = '("foo"=("foo/0"="1.1.1.1","foo/1"="1.1.1.2"))'
        targets = {'foo': {'foo/0': '1.1.1.1', 'foo/1': '1.1.1.2'}}
        json_like = to_json(targets)
        self.assertEqual(expected, json_like)

    def test_parse_targets(self):
        expected = {'foo': {'foo/0': '1.1.1.1', 'foo/1': '1.1.1.2'}}
        targets = parse_targets(apps)
        self.assertEqual(expected, targets)


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_network_health.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_network_health.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_network_health.assess_network_health",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, bundle=None,
                                            series='trusty')


class TestParseArgs(TestCase):

    def test_common_args(self):
        # Test common args
        common = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"]
        args = parse_args(common)
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)
        # Test specific args
        nh_args = parse_args([common, "--model", "foo", "--bundle", "bar"])
        self.assertEqual("foo", nh_args.model)
        self.assertEqual("bar", nh_args.bundle)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn("Network Health", fake_stdout.getvalue())
