import yaml
import json
import StringIO
import logging
import copy
from textwrap import dedent
from datetime import (
    datetime,
    timedelta,
    )
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
    AssessNetworkHealth,
    main,
    parse_args,
    _setup_spaces
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
    bindings:
        foo: foo
  bar:
    charm: local:trusty/bar
    num_units: 1
    bindings:
        bar: bar
series: trusty
relations:
- - foo:baz
  - bar:baz
""")
bundle_yaml = yaml.safe_load(bundle_string)

status_value = dedent("""\
    machines:
      "0":
        agent-state: started
        ip-addresses:
        - 1.1.1.1
        dns-name: 1.1.1.1
      "1":
        agent-state: started
        ip-addresses:
        - 1.1.1.2
        dns-name: 1.1.1.2
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
            machine: '0'
          ubuntu/1:
            subordinates:
              network-health/1:
                juju-status:
                  current: idle
                public-address: 1.1.1.2
            juju-status:
              current: idle
            public-address: 1.1.1.2
            machine: '1'
        application-status:
          current: unknown
          since: 01 Jan 2017 00:00:00-00:00
        exposed: true
        series: trusty
      network-health:
        application-status:
          current: unknown
          since: 01 Jan 2017 00:00:00-00:00
        subordinate-to:
        - ubuntu
        relations:
          juju-info:
          - network-health
        series: trusty
""")
status = Status(yaml.safe_load(status_value), status_value)

maas_spaces = dedent("""
    [{
        "subnets": [{}],
        "resource_uri": "",
        "id": 0,
        "name": "bar"
    },
    {
        "subnets": [{}],
        "resource_uri": "",
        "id": 1,
        "name": "baz"
    }]
""")
maas_spaces = json.loads(maas_spaces)

ping_result = dedent("""
results:
  results: '{u''ubuntu/0'': True, u''ubuntu/1'': True}'
status: completed
timing:
  completed: 2017-01-01 00:00:01 +0000 UTC
  enqueued: 2017-01-01 00:00:01 +0000 UTC
  started: 2017-01-01 00:00:01 +0000 UTC}
""")
curl_result = [{'foo': 'pass'}]

dummy_charm = 'dummy'
series = 'trusty'


class TestAssessNetworkHealth(TestCase):

    def test_setup_testing_environment(self):

        def setup_iteration(bundle, target_model, series):
            mock_client = Mock(spec=["juju", "wait_for_started",
                                     "wait_for_workloads", "deploy",
                                     "get_juju_output",
                                     "wait_for_subordinate_units",
                                     "get_status", "deploy_bundle"])
            mock_client.get_status.return_value = status
            mock_client.series = 'trusty'
            mock_client.version = '2.2'
            net_health.setup_testing_environment(mock_client, bundle,
                                                 target_model, series)
            return mock_client
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = setup_iteration(bundle=None, target_model=None, series=series)
        self.assertEqual(
            [call.deploy('ubuntu', num=2, series='trusty'),
             call.juju('expose', ('ubuntu',)),
             call.wait_for_started(),
             call.wait_for_workloads(),
             call.get_status(),
             call.deploy('~juju-qa/network-health',
                         alias='network-health-trusty', series='trusty'),
             call.wait_for_started(),
             call.wait_for_workloads(),
             call.juju('expose', 'network-health-trusty'),
             call.get_status(),
             call.juju('add-relation', ('ubuntu', 'network-health-trusty')),
             call.juju('add-relation', ('network-health',
                                        'network-health-trusty')),
             call.wait_for_workloads(),
             call.wait_for_subordinate_units('ubuntu',
                                             'network-health-trusty'),
             call.wait_for_subordinate_units('network-health',
                                             'network-health-trusty')],
            client.mock_calls)
        client = setup_iteration(bundle=bundle_string, target_model=None,
                                 series=series)
        self.assertEqual(
            [call.deploy_bundle('services:\n  foo:\n    '
                                'charm: local:trusty/foo\n    '
                                'num_units: 1\n    expose: true\n    '
                                'bindings:\n        foo: foo\n  bar:\n    '
                                'charm: local:trusty/bar\n    '
                                'num_units: 1\n    bindings:\n        '
                                'bar: bar\nseries: trusty\n'
                                'relations:\n- - foo:baz\n  - bar:baz\n'),
             call.wait_for_started(),
             call.wait_for_workloads(),
             call.get_status(),
             call.deploy('~juju-qa/network-health',
                         alias='network-health-trusty', series='trusty'),
             call.wait_for_started(),
             call.wait_for_workloads(),
             call.juju('expose', 'network-health-trusty'),
             call.get_status(),
             call.juju('add-relation', ('ubuntu', 'network-health-trusty')),
             call.juju('add-relation', ('network-health',
                                        'network-health-trusty')),
             call.wait_for_workloads(),
             call.wait_for_subordinate_units('ubuntu',
                                             'network-health-trusty'),
             call.wait_for_subordinate_units('network-health',
                                             'network-health-trusty')],
            client.mock_calls)

    def test_connect_to_existing_model_when_different(self):
        model = {'bar': 'baz'}
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = Mock(spec=["juju", "show_model", "switch"])
        with patch.object(client, 'show_model', return_value=model):
            net_health.connect_to_existing_model(client, 'foo')
        self.assertEqual([call.switch('foo')],
                         client.mock_calls)

    def test_connect_to_existing_model_when_same(self):
        model = {'foo': 'baz'}
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = Mock(spec=["juju", "show_model", "switch"])
        with patch.object(client, 'show_model', return_value=model):
            net_health.connect_to_existing_model(client, 'foo')
        self.assertEqual([], client.mock_calls)

    def test_reboot_machines(self):
        # TODO: Skip for now, test runs into timeout issues
        pass

    def test_neighbor_visibility(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                with patch.object(client, 'run', return_value=curl_result):
                    client.deploy('ubuntu', num=2, series='trusty')
                    client.deploy('network-health', series='trusty')
                    out = net_health.neighbor_visibility(client)
        expected = {'network-health': {},
                    'ubuntu': {'ubuntu/0': {'1.1.1.1': True, '1.1.1.2': True},
                               'ubuntu/1': {'1.1.1.1': True, '1.1.1.2': True}}}

        self.assertEqual(expected, out)

    def test_internet_connection_with_pass(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = fake_juju_client()
        client.bootstrap()
        default = ["default via 1.1.1.1 "]

        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                with patch('subprocess.check_output', return_value=None):
                    with patch.object(client, 'run', return_value=default):
                        out = net_health.internet_connection(client)
        expected = {'1': True, '0': True}
        self.assertEqual(expected, out)

    def test_internet_connection_with_fail(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = fake_juju_client()
        client.bootstrap()
        default = ["1.0.0.1/24 dev lxdbr0 "]

        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_status', return_value=status):
                with patch.object(client, 'run', return_value=default):
                    out = net_health.internet_connection(client)
        expected = {'1': False, '0': False}
        self.assertEqual(expected, out)

    def test_ensure_exposed(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        mock_client = Mock(spec=["juju", "deploy",
                                 "wait_for_subordinate_units",
                                 "get_status"])
        mock_client.get_status.return_value = status
        mock_client.series = 'xenial'
        mock_client.version = '2.2'
        with patch('subprocess.check_output', return_value='pass'):
            net_health.ensure_exposed(mock_client, series)
        self.assertEqual(
            [call.get_status(),
             call.get_status(),
             call.deploy('~juju-qa/network-health',
                         alias='network-health-ubuntu', series='trusty'),
             call.juju('add-relation', ('ubuntu', 'network-health-ubuntu')),
             call.wait_for_subordinate_units('ubuntu',
                                             'network-health-ubuntu'),
             call.juju('expose', 'network-health-ubuntu'),
             call.get_status()],
            mock_client.mock_calls)

    def test_dummy_deployment(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        net_health.setup_dummy_deployment(client, series)
        client.deploy.assert_called_once_with('ubuntu', num=2, series='trusty')

    def test_bundle_deployment(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = Mock(wraps=fake_juju_client())
        client.bootstrap()
        net_health.setup_bundle_deployment(client, bundle_string)
        client.deploy_bundle.assert_called_once_with(bundle_string)

    def test_setup_spaces_existing_spaces(self):
        existing_spaces = maas_spaces
        new_spaces = _setup_spaces(bundle_yaml, existing_spaces)
        expected_spaces = ['foo']
        self.assertEqual(expected_spaces, new_spaces)

    def test_setup_spaces_no_existing_spaces(self):
        existing_spaces = {}
        new_spaces = _setup_spaces(bundle_yaml, existing_spaces)
        expected_spaces = ['foo', 'bar']
        self.assertEqual(expected_spaces, new_spaces)

    def test_setup_spaces_existing_correct(self):
        existing_spaces = maas_spaces
        new_bundle = copy.deepcopy(bundle_yaml)
        new_bundle['services']['foo']['bindings'] = {'fizz': 'baz'}
        new_spaces = _setup_spaces(new_bundle, existing_spaces)
        expected_spaces = []
        self.assertEqual(expected_spaces, new_spaces)

    def test_parse_expose_results(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        exposed = ['bar', 'baz']
        service_results = {'foo': False,
                           'bar': True,
                           'baz': False}
        expected = {'fail': ('baz',), 'pass': ('foo', 'bar')}
        result = net_health.parse_expose_results(service_results, exposed)
        self.assertEqual(expected, result)

    def test_parse_final_results_with_fail(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        visible = {"bar/0": {"foo": {"foo/0": False, "foo/1": True}}}
        internet = {"0": False, "1": True}
        exposed = {"fail": ("foo"), "pass": ("bar", "baz")}
        out = net_health.parse_final_results(visible, internet,
                                             exposed)
        error_strings = ["Machine 0 failed internet connection.",
                         "Unit bar/0 failed to contact targets(s): "
                         "['foo/0']",
                         "Application(s) foo failed expose test"]
        for line in out:
            self.assertTrue(line in error_strings)

    def test_parse_final_results_without_fail(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        visible = {"bar/0": {"foo": {"foo/0": True, "foo/1": True}}}
        internet = {"0": True, "1": True}
        exposed = {"fail": (), "pass": ("foo", "bar", "baz")}
        net_health.parse_final_results(visible, internet, exposed)

    def test_ping_units(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = fake_juju_client()
        client.bootstrap()
        client._backend.set_action_result('bar/0', 'ping', ping_result)
        targets = {'foo/0': '1.1.1.1'}
        out = net_health.ping_units(client, 'bar/0', targets)
        result = yaml.safe_load(ping_result)
        self.assertEqual(out, result['results']['results'])

    def test_to_json(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        expected = '("foo"=("foo/0"="1.1.1.1","foo/1"="1.1.1.2"))'
        targets = {'foo': {'foo/0': '1.1.1.1', 'foo/1': '1.1.1.2'}}
        json_like = net_health.to_json(targets)
        self.assertEqual(expected, json_like)

    def test_parse_targets(self):
        args = parse_args([])
        net_health = AssessNetworkHealth(args)
        client = fake_juju_client()
        client.bootstrap()
        client.deploy('foo', num=2)
        expected = {'ubuntu': {'ubuntu/0': '1.1.1.1', 'ubuntu/1': '1.1.1.2'}}
        with patch.object(client, 'get_status', return_value=status):
            targets = net_health.parse_targets(client.get_status())
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
                    with patch('assess_network_health.'
                               'AssessNetworkHealth') as mock_anh:
                        with patch("assess_network_health.AssessNetworkHealth."
                                   "assess_network_health",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(mock_anh, client, bundle=None,
                                            reboot=False, series='trusty')


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
