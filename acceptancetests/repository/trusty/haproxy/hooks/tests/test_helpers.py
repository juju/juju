import base64
import os

from contextlib import contextmanager
from StringIO import StringIO

from testtools import TestCase
from mock import patch, call, MagicMock

import hooks
from utils_for_tests import patch_open


class HelpersTest(TestCase):

    @patch('hooks.has_ssl_support')
    @patch('hooks.config_get')
    def test_creates_haproxy_globals(self, config_get, has_ssl_support):
        config_get.return_value = {
            'global_log': 'foo-log, bar-log',
            'global_maxconn': 123,
            'global_user': 'foo-user',
            'global_group': 'foo-group',
            'global_spread_checks': 234,
            'global_default_dh_param': 345,
            'global_default_bind_ciphers': "my:ciphers",
            'global_debug': False,
            'global_quiet': False,
            'global_stats_socket': True,
        }
        has_ssl_support.return_value = True
        result = hooks.create_haproxy_globals()

        sock_path = "/var/run/haproxy/haproxy.sock"
        expected = '\n'.join([
            'global',
            '    log foo-log',
            '    log bar-log',
            '    maxconn 123',
            '    user foo-user',
            '    group foo-group',
            '    spread-checks 234',
            '    tune.ssl.default-dh-param 345',
            '    ssl-default-bind-ciphers my:ciphers',
            '    stats socket %s mode 0600' % sock_path,
        ])
        self.assertEqual(result, expected)

    @patch('hooks.has_ssl_support')
    @patch('hooks.config_get')
    def test_creates_haproxy_globals_quietly_with_debug(
            self, config_get, has_ssl_support):
        config_get.return_value = {
            'global_log': 'foo-log, bar-log',
            'global_maxconn': 123,
            'global_user': 'foo-user',
            'global_group': 'foo-group',
            'global_spread_checks': 234,
            'global_default_dh_param': 345,
            'global_default_bind_ciphers': "my:ciphers",
            'global_debug': True,
            'global_quiet': True,
            'global_stats_socket': False,
        }
        has_ssl_support.return_value = True
        result = hooks.create_haproxy_globals()

        expected = '\n'.join([
            'global',
            '    log foo-log',
            '    log bar-log',
            '    maxconn 123',
            '    user foo-user',
            '    group foo-group',
            '    debug',
            '    quiet',
            '    spread-checks 234',
            '    tune.ssl.default-dh-param 345',
            '    ssl-default-bind-ciphers my:ciphers',
        ])
        self.assertEqual(result, expected)

    @patch('hooks.has_ssl_support')
    @patch('hooks.config_get')
    def test_creates_haproxy_globals_no_ssl_support(
            self, config_get, has_ssl_support):
        config_get.return_value = {
            'global_log': 'foo-log, bar-log',
            'global_maxconn': 123,
            'global_user': 'foo-user',
            'global_group': 'foo-group',
            'global_spread_checks': 234,
            'global_debug': False,
            'global_quiet': False,
            'global_stats_socket': True,
        }
        has_ssl_support.return_value = False
        result = hooks.create_haproxy_globals()

        sock_path = "/var/run/haproxy/haproxy.sock"
        expected = '\n'.join([
            'global',
            '    log foo-log',
            '    log bar-log',
            '    maxconn 123',
            '    user foo-user',
            '    group foo-group',
            '    spread-checks 234',
            '    stats socket %s mode 0600' % sock_path,
        ])
        self.assertEqual(result, expected)

    def test_enables_haproxy(self):
        mock_file = MagicMock()

        @contextmanager
        def mock_open(*args, **kwargs):
            yield mock_file

        initial_content = """
        foo
        ENABLED=0
        bar
        """
        ending_content = initial_content.replace('ENABLED=0', 'ENABLED=1')

        with patch('__builtin__.open', mock_open):
            mock_file.read.return_value = initial_content

            hooks.enable_haproxy()

            mock_file.write.assert_called_with(ending_content)

    @patch('hooks.config_get')
    def test_creates_haproxy_defaults(self, config_get):
        config_get.return_value = {
            'default_options': 'foo-option, bar-option',
            'default_timeouts': '234, 456',
            'default_log': 'foo-log',
            'default_mode': 'foo-mode',
            'default_retries': 321,
        }
        result = hooks.create_haproxy_defaults()

        expected = '\n'.join([
            'defaults',
            '    log foo-log',
            '    mode foo-mode',
            '    option foo-option',
            '    option bar-option',
            '    retries 321',
            '    timeout 234',
            '    timeout 456',
        ])
        self.assertEqual(result, expected)

    def test_returns_none_when_haproxy_config_doesnt_exist(self):
        self.assertIsNone(hooks.load_haproxy_config('/some/foo/file'))

    @patch('__builtin__.open')
    @patch('os.path.isfile')
    def test_loads_haproxy_config_file(self, isfile, mock_open):
        content = 'some content'
        config_file = '/etc/haproxy/haproxy.cfg'
        file_object = StringIO(content)
        isfile.return_value = True
        mock_open.return_value = file_object

        result = hooks.load_haproxy_config()

        self.assertEqual(result, content)
        isfile.assert_called_with(config_file)
        mock_open.assert_called_with(config_file)

    @patch('hooks.load_haproxy_config')
    def test_gets_monitoring_password(self, load_haproxy_config):
        load_haproxy_config.return_value = 'stats auth foo:bar'

        password = hooks.get_monitoring_password()

        self.assertEqual(password, 'bar')

    @patch('hooks.load_haproxy_config')
    def test_gets_none_if_different_pattern(self, load_haproxy_config):
        load_haproxy_config.return_value = 'some other pattern'

        password = hooks.get_monitoring_password()

        self.assertIsNone(password)

    def test_gets_none_pass_if_config_doesnt_exist(self):
        password = hooks.get_monitoring_password('/some/foo/path')

        self.assertIsNone(password)

    @patch('hooks.load_haproxy_config')
    def test_gets_service_ports(self, load_haproxy_config):
        load_haproxy_config.return_value = '''
        listen foo.internal 1.2.3.4:123
        listen bar.internal 1.2.3.5:234
        '''

        ports = hooks.get_service_ports()

        self.assertEqual(ports, (123, 234))

    @patch('hooks.load_haproxy_config')
    def test_get_listen_stanzas(self, load_haproxy_config):
        load_haproxy_config.return_value = '''
        listen   foo.internal  1.2.3.4:123
        listen bar.internal    1.2.3.5:234
        '''

        stanzas = hooks.get_listen_stanzas()

        self.assertEqual((('foo.internal', '1.2.3.4', 123),
                          ('bar.internal', '1.2.3.5', 234)),
                         stanzas)

    @patch('hooks.load_haproxy_config')
    def test_get_listen_stanzas_with_frontend(self, load_haproxy_config):
        load_haproxy_config.return_value = '''
        frontend foo-2-123
            bind 1.2.3.4:123
            default_backend foo.internal
        frontend foo-2-234
            bind 1.2.3.5:234
            default_backend bar.internal
        '''

        stanzas = hooks.get_listen_stanzas()

        self.assertEqual((('foo.internal', '1.2.3.4', 123),
                          ('bar.internal', '1.2.3.5', 234)),
                         stanzas)

    @patch('hooks.load_haproxy_config')
    def test_get_listen_stanzas_with_ssl_frontend(self, load_haproxy_config):
        load_haproxy_config.return_value = '''
        frontend foo-2-123
            bind 1.2.3.4:123 ssl crt /foo/bar no-sslv3
            default_backend foo.internal
        frontend foo-2-234
            bind 1.2.3.5:234
            default_backend bar.internal
        '''

        stanzas = hooks.get_listen_stanzas()

        self.assertEqual((('foo.internal', '1.2.3.4', 123),
                          ('bar.internal', '1.2.3.5', 234)),
                         stanzas)

    @patch('hooks.load_haproxy_config')
    def test_get_empty_tuple_when_no_stanzas(self, load_haproxy_config):
        load_haproxy_config.return_value = '''
        '''

        stanzas = hooks.get_listen_stanzas()

        self.assertEqual((), stanzas)

    @patch('hooks.load_haproxy_config')
    def test_get_listen_stanzas_none_configured(self, load_haproxy_config):
        load_haproxy_config.return_value = ""

        stanzas = hooks.get_listen_stanzas()

        self.assertEqual((), stanzas)

    def test_gets_no_ports_if_config_doesnt_exist(self):
        ports = hooks.get_service_ports('/some/foo/path')
        self.assertEqual((), ports)

    @patch('hooks.open_port')
    @patch('hooks.close_port')
    def test_updates_service_ports(self, close_port, open_port):
        old_service_ports = [123, 234, 345]
        new_service_ports = [345, 456, 567]

        hooks.update_service_ports(old_service_ports, new_service_ports)

        self.assertEqual(close_port.mock_calls, [call(123), call(234)])
        self.assertEqual(open_port.mock_calls,
                         [call(456), call(567)])

    @patch('hooks.open_port')
    @patch('hooks.close_port')
    def test_updates_none_if_service_ports_not_provided(self, close_port,
                                                        open_port):
        hooks.update_service_ports()

        self.assertFalse(close_port.called)
        self.assertFalse(open_port.called)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_creates_a_listen_stanza(self):
        service_name = 'some-name'
        service_ip = '10.11.12.13'
        service_port = 1234
        service_options = ('foo', 'bar')
        server_entries = [
            ('name-1', 'ip-1', 'port-1', ('foo1', 'bar1')),
            ('name-2', 'ip-2', 'port-2', ('foo2', 'bar2')),
        ]

        result = hooks.create_listen_stanza(service_name, service_ip,
                                            service_port, service_options,
                                            server_entries)

        expected = '\n'.join((
            'frontend haproxy-2-1234',
            '    bind 10.11.12.13:1234',
            '    default_backend some-name',
            '',
            'backend some-name',
            '    foo',
            '    bar',
            '    server name-1 ip-1:port-1 foo1 bar1',
            '    server name-2 ip-2:port-2 foo2 bar2',
        ))

        self.assertEqual(expected, result)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_creates_a_listen_stanza_string_server_options(self):
        service_name = 'some-name'
        service_ip = '10.11.12.13'
        service_port = 1234
        service_options = ('foo', 'bar')
        server_entries = [
            ('name-1', 'ip-1', 'port-1', 'foo1 bar1'),
            ('name-2', 'ip-2', 'port-2', 'foo2 bar2'),
        ]

        result = hooks.create_listen_stanza(service_name, service_ip,
                                            service_port, service_options,
                                            server_entries)

        expected = '\n'.join((
            'frontend haproxy-2-1234',
            '    bind 10.11.12.13:1234',
            '    default_backend some-name',
            '',
            'backend some-name',
            '    foo',
            '    bar',
            '    server name-1 ip-1:port-1 foo1 bar1',
            '    server name-2 ip-2:port-2 foo2 bar2',
        ))

        self.assertEqual(expected, result)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_create_listen_stanza_filters_frontend_options(self):
        service_name = 'some-name'
        service_ip = '10.11.12.13'
        service_port = 1234
        service_options = ('capture request header X-Man', 'mode http',
                           'option httplog', 'retries 3', 'balance uri',
                           'option logasap')
        server_entries = [
            ('name-1', 'ip-1', 'port-1', ('foo1', 'bar1')),
            ('name-2', 'ip-2', 'port-2', ('foo2', 'bar2')),
        ]

        result = hooks.create_listen_stanza(service_name, service_ip,
                                            service_port, service_options,
                                            server_entries)

        expected = '\n'.join((
            'frontend haproxy-2-1234',
            '    bind 10.11.12.13:1234',
            '    default_backend some-name',
            '    mode http',
            '    option httplog',
            '    capture request header X-Man',
            '    option logasap',
            '',
            'backend some-name',
            '    mode http',
            '    option httplog',
            '    retries 3',
            '    balance uri',
            '    server name-1 ip-1:port-1 foo1 bar1',
            '    server name-2 ip-2:port-2 foo2 bar2',
        ))

        self.assertEqual(expected, result)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_creates_a_listen_stanza_with_tuple_entries(self):
        service_name = 'some-name'
        service_ip = '10.11.12.13'
        service_port = 1234
        service_options = ('foo', 'bar')
        server_entries = (
            ('name-1', 'ip-1', 'port-1', ('foo1', 'bar1')),
            ('name-2', 'ip-2', 'port-2', ('foo2', 'bar2')),
        )

        result = hooks.create_listen_stanza(service_name, service_ip,
                                            service_port, service_options,
                                            server_entries)

        expected = '\n'.join((
            'frontend haproxy-2-1234',
            '    bind 10.11.12.13:1234',
            '    default_backend some-name',
            '',
            'backend some-name',
            '    foo',
            '    bar',
            '    server name-1 ip-1:port-1 foo1 bar1',
            '    server name-2 ip-2:port-2 foo2 bar2',
        ))

        self.assertEqual(expected, result)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_creates_a_listen_stanza_with_errorfiles(self):
        service_name = 'some-name'
        service_ip = '10.11.12.13'
        service_port = 1234
        service_options = ('foo', 'bar')
        server_entries = [
            ('name-1', 'ip-1', 'port-1', ('foo1', 'bar1')),
            ('name-2', 'ip-2', 'port-2', ('foo2', 'bar2')),
        ]
        content = ("HTTP/1.0 403 Forbidden\r\n"
                   "Content-Type: text/html\r\n"
                   "\r\n"
                   "<html></html>")
        errorfiles = [{'http_status': 403,
                       'content': base64.b64encode(content)}]

        result = hooks.create_listen_stanza(service_name, service_ip,
                                            service_port, service_options,
                                            server_entries, errorfiles)

        expected = '\n'.join((
            'frontend haproxy-2-1234',
            '    bind 10.11.12.13:1234',
            '    default_backend some-name',
            '',
            'backend some-name',
            '    foo',
            '    bar',
            '    errorfile 403 /var/lib/haproxy/service_some-name/403.http',
            '    server name-1 ip-1:port-1 foo1 bar1',
            '    server name-2 ip-2:port-2 foo2 bar2',
        ))

        self.assertEqual(expected, result)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_creates_a_listen_stanza_with_crts(self):
        service_name = 'foo'
        service_ip = '1.2.3.4'
        service_port = 443
        server_entries = [
            ('name-1', 'ip-1', 'port-1', ('foo1', 'bar1')),
        ]
        content = ("-----BEGIN CERTIFICATE-----\n"
                   "<data>\n"
                   "-----END CERTIFICATE-----\n")
        crts = [base64.b64encode(content)]

        result = hooks.create_listen_stanza(service_name, service_ip,
                                            service_port,
                                            server_entries=server_entries,
                                            service_crts=crts)

        crt_path = '/var/lib/haproxy/service_foo/0.pem'
        expected = '\n'.join((
            'frontend haproxy-2-443',
            '    bind 1.2.3.4:443 ssl crt %s no-sslv3' % crt_path,
            '    default_backend foo',
            '',
            'backend foo',
            '    server name-1 ip-1:port-1 foo1 bar1',
        ))

        self.assertEqual(expected, result)

    @patch.dict(os.environ, {"JUJU_UNIT_NAME": "haproxy/2"})
    def test_creates_a_listen_stanza_with_backends(self):
        service_name = 'foo'
        service_ip = '1.2.3.4'
        service_port = 80
        server_entries = [
            ('name-1', 'ip-1', 'port-1', ('foo1', 'bar1')),
        ]
        service_backends = [
            {"backend_name": "foo-bar",
             "servers": [
                 ('bar-name-1', 'bar-ip-1', 'bar-port-1', ('bar2', 'bar3'))
             ]}
        ]
        result = hooks.create_listen_stanza(
            service_name, service_ip, service_port,
            server_entries=server_entries, service_backends=service_backends)

        expected = '\n'.join((
            'frontend haproxy-2-80',
            '    bind 1.2.3.4:80',
            '    default_backend foo',
            '',
            'backend foo',
            '    server name-1 ip-1:port-1 foo1 bar1',
            '',
            'backend foo-bar',
            '    server bar-name-1 bar-ip-1:bar-port-1 bar2 bar3',
        ))

        self.assertEqual(expected, result)

    def test_doesnt_create_listen_stanza_if_args_not_provided(self):
        self.assertIsNone(hooks.create_listen_stanza())

    @patch('hooks.create_listen_stanza')
    @patch('hooks.config_get')
    @patch('hooks.get_monitoring_password')
    def test_creates_a_monitoring_stanza(self, get_monitoring_password,
                                         config_get, create_listen_stanza):
        config_get.return_value = {
            'enable_monitoring': True,
            'monitoring_allowed_cidr': 'some-cidr',
            'monitoring_password': 'some-pass',
            'monitoring_username': 'some-user',
            'monitoring_stats_refresh': 123,
            'monitoring_port': 1234,
        }
        create_listen_stanza.return_value = 'some result'

        result = hooks.create_monitoring_stanza(service_name="some-service")

        self.assertEqual('some result', result)
        get_monitoring_password.assert_called_with()
        create_listen_stanza.assert_called_with(
            'some-service', '0.0.0.0', 1234, [
                'mode http',
                'acl allowed_cidr src some-cidr',
                'http-request deny unless allowed_cidr',
                'stats enable',
                'stats uri /',
                'stats realm Haproxy\\ Statistics',
                'stats auth some-user:some-pass',
                'stats refresh 123',
            ])

    @patch('hooks.create_listen_stanza')
    @patch('hooks.config_get')
    @patch('hooks.get_monitoring_password')
    def test_doesnt_create_a_monitoring_stanza_if_monitoring_disabled(
            self, get_monitoring_password, config_get, create_listen_stanza):
        config_get.return_value = {
            'enable_monitoring': False,
        }

        result = hooks.create_monitoring_stanza(service_name="some-service")

        self.assertIsNone(result)
        self.assertFalse(get_monitoring_password.called)
        self.assertFalse(create_listen_stanza.called)

    @patch('hooks.create_listen_stanza')
    @patch('hooks.config_get')
    @patch('hooks.get_monitoring_password')
    def test_uses_monitoring_password_for_stanza(self, get_monitoring_password,
                                                 config_get,
                                                 create_listen_stanza):
        config_get.return_value = {
            'enable_monitoring': True,
            'monitoring_allowed_cidr': 'some-cidr',
            'monitoring_password': 'changeme',
            'monitoring_username': 'some-user',
            'monitoring_stats_refresh': 123,
            'monitoring_port': 1234,
        }
        create_listen_stanza.return_value = 'some result'
        get_monitoring_password.return_value = 'some-monitoring-pass'

        hooks.create_monitoring_stanza(service_name="some-service")

        get_monitoring_password.assert_called_with()
        create_listen_stanza.assert_called_with(
            'some-service', '0.0.0.0', 1234, [
                'mode http',
                'acl allowed_cidr src some-cidr',
                'http-request deny unless allowed_cidr',
                'stats enable',
                'stats uri /',
                'stats realm Haproxy\\ Statistics',
                'stats auth some-user:some-monitoring-pass',
                'stats refresh 123',
            ])

    @patch('hooks.pwgen')
    @patch('hooks.create_listen_stanza')
    @patch('hooks.config_get')
    @patch('hooks.get_monitoring_password')
    def test_uses_new_password_for_stanza(self, get_monitoring_password,
                                          config_get, create_listen_stanza,
                                          pwgen):
        config_get.return_value = {
            'enable_monitoring': True,
            'monitoring_allowed_cidr': 'some-cidr',
            'monitoring_password': 'changeme',
            'monitoring_username': 'some-user',
            'monitoring_stats_refresh': 123,
            'monitoring_port': 1234,
        }
        create_listen_stanza.return_value = 'some result'
        get_monitoring_password.return_value = None
        pwgen.return_value = 'some-new-pass'

        hooks.create_monitoring_stanza(service_name="some-service")

        get_monitoring_password.assert_called_with()
        create_listen_stanza.assert_called_with(
            'some-service', '0.0.0.0', 1234, [
                'mode http',
                'acl allowed_cidr src some-cidr',
                'http-request deny unless allowed_cidr',
                'stats enable',
                'stats uri /',
                'stats realm Haproxy\\ Statistics',
                'stats auth some-user:some-new-pass',
                'stats refresh 123',
            ])

    @patch('hooks.is_proxy')
    @patch('hooks.config_get')
    @patch('yaml.safe_load')
    def test_gets_config_services(self, safe_load, config_get, is_proxy):
        config_get.return_value = {
            'services': 'some-services',
        }
        safe_load.return_value = [
            {
                'service_name': 'foo',
                'service_options': {
                    'foo-1': 123,
                },
                'service_options': ['foo1', 'foo2'],
                'server_options': ['baz1', 'baz2'],
            },
            {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2'],
                'server_options': ['baz1', 'baz2'],
            },
        ]
        is_proxy.return_value = False

        result = hooks.get_config_services()
        expected = {
            None: {
                'service_name': 'foo',
            },
            'foo': {
                'service_name': 'foo',
                'service_options': ['foo1', 'foo2'],
                'server_options': ['baz1', 'baz2'],
            },
            'bar': {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2'],
                'server_options': ['baz1', 'baz2'],
            },
        }

        self.assertEqual(expected, result)

    @patch('hooks.is_proxy')
    @patch('hooks.config_get')
    @patch('yaml.safe_load')
    def test_gets_config_services_with_forward_option(self, safe_load,
                                                      config_get, is_proxy):
        config_get.return_value = {
            'services': 'some-services',
        }
        safe_load.return_value = [
            {
                'service_name': 'foo',
                'service_options': {
                    'foo-1': 123,
                },
                'service_options': ['foo1', 'foo2'],
                'server_options': ['baz1', 'baz2'],
            },
            {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2'],
                'server_options': ['baz1', 'baz2'],
            },
        ]
        is_proxy.return_value = True

        result = hooks.get_config_services()
        expected = {
            None: {
                'service_name': 'foo',
            },
            'foo': {
                'service_name': 'foo',
                'service_options': ['foo1', 'foo2', 'option forwardfor'],
                'server_options': ['baz1', 'baz2'],
            },
            'bar': {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2', 'option forwardfor'],
                'server_options': ['baz1', 'baz2'],
            },
        }

        self.assertEqual(expected, result)

    @patch('hooks.is_proxy')
    @patch('hooks.config_get')
    @patch('yaml.safe_load')
    def test_gets_config_services_with_options_string(self, safe_load,
                                                      config_get, is_proxy):
        config_get.return_value = {
            'services': 'some-services',
        }
        safe_load.return_value = [
            {
                'service_name': 'foo',
                'service_options': {
                    'foo-1': 123,
                },
                'service_options': ['foo1', 'foo2'],
                'server_options': 'baz1, baz2',
            },
            {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2'],
                'server_options': 'baz1, baz2',
            },
        ]
        is_proxy.return_value = False

        result = hooks.get_config_services()
        expected = {
            None: {
                'service_name': 'foo',
            },
            'foo': {
                'service_name': 'foo',
                'service_options': ['foo1', 'foo2'],
                'server_options': ['baz1', 'baz2'],
            },
            'bar': {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2'],
                'server_options': ['baz1', 'baz2'],
            },
        }

        self.assertEqual(expected, result)

    @patch('hooks.is_proxy')
    @patch('hooks.config_get')
    @patch('yaml.safe_load')
    def test_gets_config_services_with_proxy_no_forward(self, safe_load,
                                                        config_get, is_proxy):
        config_get.return_value = {
            'services': 'some-services',
        }
        safe_load.return_value = [
            {
                'service_name': 'foo',
                'service_options': {
                    'foo-1': 123,
                },
                'service_options': ['foo1', 'foo2'],
                'server_options': 'baz1, baz2',
            },
            {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2'],
                'server_options': 'baz1, baz2',
            },
        ]
        is_proxy.return_value = True

        result = hooks.get_config_services()
        expected = {
            None: {
                'service_name': 'foo',
            },
            'foo': {
                'service_name': 'foo',
                'service_options': ['foo1', 'foo2', 'option forwardfor'],
                'server_options': ['baz1', 'baz2'],
            },
            'bar': {
                'service_name': 'bar',
                'service_options': ['bar1', 'bar2', 'option forwardfor'],
                'server_options': ['baz1', 'baz2'],
            },
        }

        self.assertEqual(expected, result)

    @patch('hooks.is_proxy')
    @patch('hooks.config_get')
    @patch('yaml.safe_load')
    def test_gets_config_services_no_service_options(self, safe_load,
                                                     config_get, is_proxy):
        config_get.return_value = {
            'services': '',
        }
        safe_load.return_value = [
            {
                'service_name': 'foo',
                'server_options': 'baz1, baz2',
            },
            {
                'service_name': 'bar',
                'server_options': 'baz1, baz2',
            },
        ]
        is_proxy.return_value = True

        result = hooks.get_config_services()
        expected = {
            None: {
                'service_name': 'foo',
            },
            'foo': {
                'service_name': 'foo',
                'server_options': ['baz1', 'baz2'],
            },
            'bar': {
                'service_name': 'bar',
                'server_options': ['baz1', 'baz2'],
            },
        }

        self.assertEqual(expected, result)

    @patch('hooks.get_config_services')
    def test_gets_a_service_config(self, get_config_services):
        get_config_services.return_value = {
            'foo': 'bar',
        }

        self.assertEqual('bar', hooks.get_config_service('foo'))

    @patch('hooks.get_config_services')
    def test_gets_a_service_config_from_none(self, get_config_services):
        get_config_services.return_value = {
            None: 'bar',
        }

        self.assertEqual('bar', hooks.get_config_service())

    @patch('hooks.get_config_services')
    def test_gets_a_service_config_as_none(self, get_config_services):
        get_config_services.return_value = {
            'baz': 'bar',
        }

        self.assertIsNone(hooks.get_config_service())

    @patch('os.path.exists')
    def test_mark_as_proxy_when_path_exists(self, path_exists):
        path_exists.return_value = True

        self.assertTrue(hooks.is_proxy('foo'))
        path_exists.assert_called_with('/var/run/haproxy/foo.is.proxy')

    @patch('os.path.exists')
    def test_doesnt_mark_as_proxy_when_path_doesnt_exist(self, path_exists):
        path_exists.return_value = False

        self.assertFalse(hooks.is_proxy('foo'))
        path_exists.assert_called_with('/var/run/haproxy/foo.is.proxy')

    @patch('os.path.exists')
    def test_loads_services_by_name(self, path_exists):
        with patch_open() as (mock_open, mock_file):
            path_exists.return_value = True
            mock_file.read.return_value = 'some content'

            result = hooks.load_services('some-service')

            self.assertEqual('some content', result)
            mock_open.assert_called_with(
                '/var/run/haproxy/some-service.service')
            mock_file.read.assert_called_with()

    @patch('os.path.exists')
    def test_loads_no_service_if_path_doesnt_exist(self, path_exists):
        path_exists.return_value = False

        result = hooks.load_services('some-service')

        self.assertIsNone(result)

    @patch('glob.glob')
    def test_loads_services_within_dir_if_no_name_provided(self, glob):
        with patch_open() as (mock_open, mock_file):
            mock_file.read.side_effect = ['foo', 'bar']
            glob.return_value = ['foo-file', 'bar-file']

            result = hooks.load_services()

            self.assertEqual('foo\n\nbar\n\n', result)
            mock_open.assert_has_calls([call('foo-file'), call('bar-file')])
            mock_file.read.assert_has_calls([call(), call()])

    @patch('hooks.os')
    def test_removes_services_by_name(self, os_):
        service_path = '/var/run/haproxy/some-service.service'
        os_.path.exists.return_value = True

        self.assertTrue(hooks.remove_services('some-service'))

        os_.path.exists.assert_called_with(service_path)
        os_.remove.assert_called_with(service_path)

    @patch('hooks.os')
    def test_removes_nothing_if_service_doesnt_exist(self, os_):
        service_path = '/var/run/haproxy/some-service.service'
        os_.path.exists.return_value = False

        self.assertTrue(hooks.remove_services('some-service'))

        os_.path.exists.assert_called_with(service_path)

    @patch('hooks.os')
    @patch('glob.glob')
    def test_removes_all_services_in_dir_if_name_not_provided(self, glob, os_):
        glob.return_value = ['foo', 'bar']

        self.assertTrue(hooks.remove_services())

        os_.remove.assert_has_calls([call('foo'), call('bar')])

    @patch('hooks.os')
    @patch('hooks.log')
    def test_logs_error_when_failing_to_remove_service_by_name(self, log, os_):
        error = Exception('some error')
        os_.path.exists.return_value = True
        os_.remove.side_effect = error

        self.assertFalse(hooks.remove_services('some-service'))

        log.assert_called_with(str(error))

    @patch('hooks.os')
    @patch('hooks.log')
    @patch('glob.glob')
    def test_logs_error_when_failing_to_remove_services(self, glob, log, os_):
        errors = [Exception('some error 1'), Exception('some error 2')]
        os_.remove.side_effect = errors
        glob.return_value = ['foo', 'bar']

        self.assertTrue(hooks.remove_services())

        log.assert_has_calls([
            call(str(errors[0])),
            call(str(errors[1])),
        ])

    @patch('subprocess.call')
    def test_calls_check_action(self, mock_call):
        mock_call.return_value = 0

        result = hooks.service_haproxy('check')

        self.assertTrue(result)
        mock_call.assert_called_with(['/usr/sbin/haproxy', '-f',
                                      hooks.default_haproxy_config, '-c'])

    @patch('subprocess.call')
    def test_calls_check_action_with_different_config(self, mock_call):
        mock_call.return_value = 0

        result = hooks.service_haproxy('check', 'some-config')

        self.assertTrue(result)
        mock_call.assert_called_with(['/usr/sbin/haproxy', '-f',
                                      'some-config', '-c'])

    @patch('subprocess.call')
    def test_fails_to_check_config(self, mock_call):
        mock_call.return_value = 1

        result = hooks.service_haproxy('check')

        self.assertFalse(result)

    @patch('subprocess.call')
    def test_calls_different_actions(self, mock_call):
        mock_call.return_value = 0

        result = hooks.service_haproxy('foo')

        self.assertTrue(result)
        mock_call.assert_called_with(['service', 'haproxy', 'foo'])

    @patch('subprocess.call')
    def test_fails_to_call_different_actions(self, mock_call):
        mock_call.return_value = 1

        result = hooks.service_haproxy('foo')

        self.assertFalse(result)

    @patch('subprocess.call')
    def test_doesnt_call_actions_if_action_not_provided(self, mock_call):
        self.assertIsNone(hooks.service_haproxy())
        self.assertFalse(mock_call.called)

    @patch('subprocess.call')
    def test_doesnt_call_actions_if_config_is_none(self, mock_call):
        self.assertIsNone(hooks.service_haproxy('foo', None))
        self.assertFalse(mock_call.called)
