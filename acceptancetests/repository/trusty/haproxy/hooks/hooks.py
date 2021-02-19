#!/usr/bin/env python

import base64
import glob
import os
import re
import socket
import shutil
import subprocess
import sys
import yaml
import pwd

from itertools import izip, tee
from operator import itemgetter

from charmhelpers.core.host import pwgen, lsb_release, service_restart
from charmhelpers.core.hookenv import (
    log,
    config as config_get,
    local_unit,
    relation_set,
    relation_ids as get_relation_ids,
    relations_of_type,
    relations_for_id,
    relation_id,
    open_port,
    close_port,
    unit_get,
    )

from charmhelpers.fetch import (
    apt_install,
    add_source,
    apt_update,
    apt_cache
)

from charmhelpers.contrib.charmsupport import nrpe


# #############################################################################
# Global variables
# #############################################################################
default_haproxy_config_dir = "/etc/haproxy"
default_haproxy_config = "%s/haproxy.cfg" % default_haproxy_config_dir
default_haproxy_service_config_dir = "/var/run/haproxy"
default_haproxy_lib_dir = "/var/lib/haproxy"
metrics_cronjob_path = "/etc/cron.d/haproxy_metrics"
metrics_script_path = "/usr/local/bin/haproxy_to_statsd.sh"
service_affecting_packages = ['haproxy']
apt_backports_template = (
    "deb http://archive.ubuntu.com/ubuntu %(release)s-backports "
    "main restricted universe multiverse")
haproxy_preferences_path = "/etc/apt/preferences.d/haproxy"


dupe_options = [
    "mode tcp",
    "option tcplog",
    "mode http",
    "option httplog",
    ]

frontend_only_options = [
    "acl",
    "backlog",
    "bind",
    "capture cookie",
    "capture request header",
    "capture response header",
    "clitimeout",
    "default_backend",
    "http-request",
    "maxconn",
    "monitor fail",
    "monitor-net",
    "monitor-uri",
    "option accept-invalid-http-request",
    "option clitcpka",
    "option contstats",
    "option dontlog-normal",
    "option dontlognull",
    "option http-use-proxy-header",
    "option log-separate-errors",
    "option logasap",
    "option socket-stats",
    "option tcp-smart-accept",
    "rate-limit sessions",
    "redirect",
    "tcp-request content accept",
    "tcp-request content reject",
    "tcp-request inspect-delay",
    "timeout client",
    "timeout clitimeout",
    "use_backend",
    ]


class InvalidRelationDataError(Exception):
    """Invalid data has been provided in the relation."""


# #############################################################################
# Supporting functions
# #############################################################################

def comma_split(value):
    values = value.split(",")
    return filter(None, (v.strip() for v in values))


def ensure_package_status(packages, status):
    if status in ['install', 'hold']:
        selections = ''.join(['{} {}\n'.format(package, status)
                              for package in packages])
        dpkg = subprocess.Popen(['dpkg', '--set-selections'],
                                stdin=subprocess.PIPE)
        dpkg.communicate(input=selections)


def render_template(template_name, vars):
    # deferred import so install hook can install jinja2
    from jinja2 import Environment, FileSystemLoader
    templates_dir = os.path.join(os.environ['CHARM_DIR'], 'templates')
    template_env = Environment(loader=FileSystemLoader(templates_dir))
    template = template_env.get_template(template_name)
    return template.render(vars)


# -----------------------------------------------------------------------------
# enable_haproxy:  Enabled haproxy at boot time
# -----------------------------------------------------------------------------
def enable_haproxy():
    default_haproxy = "/etc/default/haproxy"
    with open(default_haproxy) as f:
        enabled_haproxy = f.read().replace('ENABLED=0', 'ENABLED=1')
    with open(default_haproxy, 'w') as f:
        f.write(enabled_haproxy)


# -----------------------------------------------------------------------------
# create_haproxy_globals:  Creates the global section of the haproxy config
# -----------------------------------------------------------------------------
def create_haproxy_globals():
    config_data = config_get()
    global_log = comma_split(config_data['global_log'])
    haproxy_globals = []
    haproxy_globals.append('global')
    for global_log_item in global_log:
        haproxy_globals.append("    log %s" % global_log_item.strip())
    haproxy_globals.append("    maxconn %d" % config_data['global_maxconn'])
    haproxy_globals.append("    user %s" % config_data['global_user'])
    haproxy_globals.append("    group %s" % config_data['global_group'])
    if config_data['global_debug'] is True:
        haproxy_globals.append("    debug")
    if config_data['global_quiet'] is True:
        haproxy_globals.append("    quiet")
    haproxy_globals.append("    spread-checks %d" %
                           config_data['global_spread_checks'])
    if has_ssl_support():
        haproxy_globals.append("    tune.ssl.default-dh-param %d" %
                               config_data['global_default_dh_param'])
        haproxy_globals.append("    ssl-default-bind-ciphers %s" %
                               config_data['global_default_bind_ciphers'])
    if config_data['global_stats_socket'] is True:
        sock_path = "/var/run/haproxy/haproxy.sock"
        haproxy_globals.append("    stats socket %s mode 0600" % sock_path)
    return '\n'.join(haproxy_globals)


# -----------------------------------------------------------------------------
# create_haproxy_defaults:  Creates the defaults section of the haproxy config
# -----------------------------------------------------------------------------
def create_haproxy_defaults():
    config_data = config_get()
    default_options = comma_split(config_data['default_options'])
    default_timeouts = comma_split(config_data['default_timeouts'])
    haproxy_defaults = []
    haproxy_defaults.append("defaults")
    haproxy_defaults.append("    log %s" % config_data['default_log'])
    haproxy_defaults.append("    mode %s" % config_data['default_mode'])
    for option_item in default_options:
        haproxy_defaults.append("    option %s" % option_item.strip())
    haproxy_defaults.append("    retries %d" % config_data['default_retries'])
    for timeout_item in default_timeouts:
        haproxy_defaults.append("    timeout %s" % timeout_item.strip())
    return '\n'.join(haproxy_defaults)


# -----------------------------------------------------------------------------
# load_haproxy_config:  Convenience function that loads (as a string) the
#                       current haproxy configuration file.
#                       Returns a string containing the haproxy config or
#                       None
# -----------------------------------------------------------------------------
def load_haproxy_config(haproxy_config_file="/etc/haproxy/haproxy.cfg"):
    if os.path.isfile(haproxy_config_file):
        return open(haproxy_config_file).read()
    else:
        return None


# -----------------------------------------------------------------------------
# get_monitoring_password:  Gets the monitoring password from the
#                           haproxy config.
#                           This prevents the password from being constantly
#                           regenerated by the system.
# -----------------------------------------------------------------------------
def get_monitoring_password(haproxy_config_file="/etc/haproxy/haproxy.cfg"):
    haproxy_config = load_haproxy_config(haproxy_config_file)
    if haproxy_config is None:
        return None
    m = re.search(r"stats auth\s+(\w+):(\w+)", haproxy_config)
    if m is not None:
        return m.group(2)
    else:
        return None


# -----------------------------------------------------------------------------
# get_service_ports:  Convenience function that scans the existing haproxy
#                     configuration file and returns a list of the existing
#                     ports being used.  This is necessary to know which ports
#                     to open and close when exposing/unexposing a service
# -----------------------------------------------------------------------------
def get_service_ports(haproxy_config_file="/etc/haproxy/haproxy.cfg"):
    stanzas = get_listen_stanzas(haproxy_config_file=haproxy_config_file)
    return tuple((int(port) for service, addr, port in stanzas))


# -----------------------------------------------------------------------------
# get_listen_stanzas: Convenience function that scans the existing haproxy
#                     configuration file and returns a list of the existing
#                     listen stanzas cofnigured.
# -----------------------------------------------------------------------------
def get_listen_stanzas(haproxy_config_file="/etc/haproxy/haproxy.cfg"):
    haproxy_config = load_haproxy_config(haproxy_config_file)
    if haproxy_config is None:
        return ()
    listen_stanzas = re.findall(
        r"listen\s+([^\s]+)\s+([^:]+):(.*)",
        haproxy_config)
    # Match bind stanzas like:
    #
    # bind 1.2.3.5:234
    # bind 1.2.3.4:123 ssl crt /foo/bar
    bind_stanzas = re.findall(
        r"\s+bind\s+([^:]+):(\d+).*\n\s+default_backend\s+([^\s]+)",
        haproxy_config, re.M)
    return (tuple(((service, addr, int(port))
                   for service, addr, port in listen_stanzas)) +
            tuple(((service, addr, int(port))
                   for addr, port, service in bind_stanzas)))


# -----------------------------------------------------------------------------
# update_service_ports:  Convenience function that evaluate the old and new
#                        service ports to decide which ports need to be
#                        opened and which to close
# -----------------------------------------------------------------------------
def update_service_ports(old_service_ports=None, new_service_ports=None):
    if old_service_ports is None or new_service_ports is None:
        return None
    for port in old_service_ports:
        if port not in new_service_ports:
            close_port(port)
    for port in new_service_ports:
        if port not in old_service_ports:
            open_port(port)


# -----------------------------------------------------------------------------
# update_sysctl: create a sysctl.conf file from YAML-formatted 'sysctl' config
# -----------------------------------------------------------------------------
def update_sysctl(config_data):
    sysctl_dict = yaml.load(config_data.get("sysctl", "{}"))
    if sysctl_dict:
        sysctl_file = open("/etc/sysctl.d/50-haproxy.conf", "w")
        for key in sysctl_dict:
            sysctl_file.write("{}={}\n".format(key, sysctl_dict[key]))
        sysctl_file.close()
        subprocess.call(["sysctl", "-p", "/etc/sysctl.d/50-haproxy.conf"])


# -----------------------------------------------------------------------------
# update_ssl_cert: write the default SSL certificate using the values from the
#                 'ssl-cert'/'ssl-key' and configuration keys
# -----------------------------------------------------------------------------
def update_ssl_cert(config_data):
    ssl_cert = config_data.get("ssl_cert")
    if not ssl_cert:
        return
    if ssl_cert == "SELFSIGNED":
        log("Using self-signed certificate")
        content = "".join(get_selfsigned_cert())
    else:
        ssl_key = config_data.get("ssl_key")
        if not ssl_key:
            log("No ssl_key provided, proceeding without default certificate")
            return
        log("Using config-provided certificate")
        content = base64.b64decode(ssl_cert)
        content += base64.b64decode(ssl_key)

    pem_path = os.path.join(default_haproxy_lib_dir, "default.pem")
    write_ssl_pem(pem_path, content)


# -----------------------------------------------------------------------------
# create_listen_stanza: Function to create a generic listen section in the
#                       haproxy config
#                       service_name:  Arbitrary service name
#                       service_ip:  IP address to listen for connections
#                       service_port:  Port to listen for connections
#                       service_options:  Comma separated list of options
#                       server_entries:  List of tuples
#                                         server_name
#                                         server_ip
#                                         server_port
#                                         server_options
#                       backends:  List of dicts
#                                  backend_name: backend name,
#                                  servers: list of tuples as in server_entries
#                       errorfiles: List of dicts
#                                   http_status: status to handle
#                                   content: base 64 content for HAProxy to
#                                            write to socket
#                       crts: List of base 64 contents for SSL certificate
#                             files that will be used in the bind line.
# -----------------------------------------------------------------------------
def create_listen_stanza(service_name=None, service_ip=None,
                         service_port=None, service_options=None,
                         server_entries=None, service_errorfiles=None,
                         service_crts=None, service_backends=None):
    if service_name is None or service_ip is None or service_port is None:
        return None
    fe_options = []
    be_options = []
    if service_options is not None:
        # For options that should be duplicated in both frontend and backend,
        # copy them to both.
        for o in dupe_options:
            if any(map(o.strip().startswith, service_options)):
                fe_options.append(o)
                be_options.append(o)
        # Filter provided service options into frontend-only and backend-only.
        results = izip(
            (fe_options, be_options),
            (True, False),
            tee((o, any(map(o.strip().startswith,
                            frontend_only_options)))
                for o in service_options))
        for out, cond, result in results:
            out.extend(option for option, match in result
                       if match is cond and option not in out)
    service_config = []
    unit_name = os.environ["JUJU_UNIT_NAME"].replace("/", "-")
    service_config.append("frontend %s-%s" % (unit_name, service_port))
    bind_stanza = "    bind %s:%s" % (service_ip, service_port)
    if service_crts:
        # Enable SSL termination for this frontend, using the given
        # certificates.
        bind_stanza += " ssl"
        for i, crt in enumerate(service_crts):
            if crt == "DEFAULT":
                path = os.path.join(default_haproxy_lib_dir, "default.pem")
            else:
                path = os.path.join(default_haproxy_lib_dir,
                                    "service_%s" % service_name, "%d.pem" % i)
            # SSLv3 is always off, since it's vulnerable to POODLE attacks
            bind_stanza += " crt %s no-sslv3" % path
    service_config.append(bind_stanza)
    service_config.append("    default_backend %s" % (service_name,))
    service_config.extend("    %s" % service_option.strip()
                          for service_option in fe_options)

    # For now errorfiles are common for all backends, in the future we
    # might offer support for per-backend error files.
    backend_errorfiles = []  # List of (status, path) tuples
    if service_errorfiles is not None:
        for errorfile in service_errorfiles:
            path = os.path.join(default_haproxy_lib_dir,
                                "service_%s" % service_name,
                                "%s.http" % errorfile["http_status"])
            backend_errorfiles.append((errorfile["http_status"], path))

    # Default backend
    _append_backend(
        service_config, service_name, be_options, backend_errorfiles,
        server_entries)

    # Extra backends
    if service_backends is not None:
        for service_backend in service_backends:
            _append_backend(
                service_config, service_backend["backend_name"],
                be_options, backend_errorfiles, service_backend["servers"])

    return '\n'.join(service_config)


def _append_backend(service_config, name, options, errorfiles, server_entries):
    """Append a new backend stanza to the given service_config.

    A backend stanza consists in a 'backend <name>' line followed by option
    lines, errorfile lines and server line.
    """
    service_config.append("")
    service_config.append("backend %s" % (name,))
    service_config.extend("    %s" % option.strip() for option in options)
    for status, path in errorfiles:
        service_config.append("    errorfile %s %s" % (status, path))
    if isinstance(server_entries, (list, tuple)):
        for i, (server_name, server_ip, server_port,
                server_options) in enumerate(server_entries):
            server_line = "    server %s %s:%s" % \
                (server_name, server_ip, server_port)
            if server_options is not None:
                if isinstance(server_options, str):
                    server_line += " " + server_options
                else:
                    server_line += " " + " ".join(server_options)
            server_line = server_line.format(i=i)
            service_config.append(server_line)


# -----------------------------------------------------------------------------
# create_monitoring_stanza:  Function to create the haproxy monitoring section
#                            service_name: Arbitrary name
# -----------------------------------------------------------------------------
def create_monitoring_stanza(service_name="haproxy_monitoring"):
    config_data = config_get()
    if config_data['enable_monitoring'] is False:
        return None
    monitoring_password = get_monitoring_password()
    if config_data['monitoring_password'] != "changeme":
        monitoring_password = config_data['monitoring_password']
    elif (monitoring_password is None and
          config_data['monitoring_password'] == "changeme"):
        monitoring_password = pwgen(length=20)
    monitoring_config = []
    monitoring_config.append("mode http")
    monitoring_config.append("acl allowed_cidr src %s" %
                             config_data['monitoring_allowed_cidr'])
    monitoring_config.append("http-request deny unless allowed_cidr")
    monitoring_config.append("stats enable")
    monitoring_config.append("stats uri /")
    monitoring_config.append(r"stats realm Haproxy\ Statistics")
    monitoring_config.append("stats auth %s:%s" %
                             (config_data['monitoring_username'],
                              monitoring_password))
    monitoring_config.append("stats refresh %d" %
                             config_data['monitoring_stats_refresh'])
    return create_listen_stanza(service_name,
                                "0.0.0.0",
                                config_data['monitoring_port'],
                                monitoring_config)


# -----------------------------------------------------------------------------
# get_config_services:  Convenience function that returns a mapping containing
#                       all of the services configuration
# -----------------------------------------------------------------------------
def get_config_services():
    config_data = config_get()
    services = {}
    return parse_services_yaml(services, config_data['services'])


def parse_services_yaml(services, yaml_data):
    """
    Parse given yaml services data.  Add it into the "services" dict.  Ensure
    that you union multiple services "server" entries, as these are the haproxy
    backends that are contacted.
    """
    yaml_services = yaml.safe_load(yaml_data)
    if yaml_services is None:
        return services

    for service in yaml_services:
        service_name = service["service_name"]
        if not services:
            # 'None' is used as a marker for the first service defined, which
            # is used as the default service if a proxied server doesn't
            # specify which service it is bound to.
            services[None] = {"service_name": service_name}

        if "service_options" in service:
            if isinstance(service["service_options"], str):
                service["service_options"] = comma_split(
                    service["service_options"])

            if is_proxy(service_name) and ("option forwardfor" not in
                                           service["service_options"]):
                service["service_options"].append("option forwardfor")

        if (("server_options" in service and
             isinstance(service["server_options"], str))):
            service["server_options"] = comma_split(service["server_options"])

        services[service_name] = merge_service(
            services.get(service_name, {}), service)

    return services


def _add_items_if_missing(target, additions):
    """
    Append items from `additions` to `target` if they are not present already.

    Returns a new list.
    """
    result = target[:]
    for addition in additions:
        if addition not in result:
            result.append(addition)
    return result


def merge_service(old_service, new_service):
    """
    Helper function to merge two service entries correctly.
    Everything will get trampled (preferring old_service), except "servers"
    which will be unioned acrosss both entries, stripping strict dups.
    """
    service = new_service.copy()
    service.update(old_service)

    # Merge all 'servers' entries of the default backend.
    if "servers" in old_service and "servers" in new_service:
        service["servers"] = _add_items_if_missing(
            old_service["servers"], new_service["servers"])

    # Merge all 'backends' and their contained "servers".
    if "backends" in old_service and "backends" in new_service:
        backends_by_name = {}
        # Go through backends in old and new configs and add them to
        # backends_by_name, merging 'servers' while at it.
        for backend in service["backends"] + new_service["backends"]:
            backend_name = backend.get("backend_name")
            if backend_name is None:
                raise InvalidRelationDataError(
                    "Each backend must have backend_name.")
            if backend_name in backends_by_name:
                # Merge servers.
                target_backend = backends_by_name[backend_name]
                target_backend["servers"] = _add_items_if_missing(
                    target_backend["servers"], backend["servers"])
            else:
                backends_by_name[backend_name] = backend

        service["backends"] = sorted(
            backends_by_name.values(), key=itemgetter('backend_name'))
    return service


def ensure_service_host_port(services):
    config_data = config_get()
    seen = []
    missing = []
    for service, options in sorted(services.iteritems()):
        if "service_host" not in options:
            missing.append(options)
            continue
        if "service_port" not in options:
            missing.append(options)
            continue
        seen.append((options["service_host"], int(options["service_port"])))

    seen.sort()
    last_port = seen and seen[-1][1] or int(config_data["monitoring_port"])
    for options in missing:
        last_port += 2
        options["service_host"] = "0.0.0.0"
        options["service_port"] = last_port

    return services


# -----------------------------------------------------------------------------
# get_config_service:   Convenience function that returns a dictionary
#                       of the configuration of a given service's configuration
# -----------------------------------------------------------------------------
def get_config_service(service_name=None):
    return get_config_services().get(service_name, None)


def is_proxy(service_name):
    flag_path = os.path.join(default_haproxy_service_config_dir,
                             "%s.is.proxy" % service_name)
    return os.path.exists(flag_path)


# -----------------------------------------------------------------------------
# create_services:  Function that will create the services configuration
#                   from the config data and/or relation information
# -----------------------------------------------------------------------------
def create_services():
    services_dict = get_config_services()
    config_data = config_get()

    # Augment services_dict with service definitions from relation data.
    relation_data = relations_of_type("reverseproxy")

    # Handle relations which specify their own services clauses
    for relation_info in relation_data:
        if "services" in relation_info:
            services_dict = parse_services_yaml(services_dict,
                                                relation_info['services'])

    if len(services_dict) == 0:
        log("No services configured, exiting.")
        return

    for relation_info in relation_data:
        unit = relation_info['__unit__']

        # Skip entries that specify their own services clauses, this was
        # handled earlier.
        if "services" in relation_info:
            log("Unit '%s' overrides 'services', "
                "skipping further processing." % unit)
            continue

        juju_service_name = unit.rpartition('/')[0]

        relation_ok = True
        for required in ("port", "private-address"):
            if required not in relation_info:
                log("No %s in relation data for '%s', skipping." %
                    (required, unit))
                relation_ok = False
                break

        if not relation_ok:
            continue

        # Mandatory switches ( private-address, port )
        host = relation_info['private-address']
        port = relation_info['port']
        server_name = ("%s-%s" % (unit.replace("/", "-"), port))

        # Optional switches ( service_name, sitenames )
        service_names = set()
        if 'service_name' in relation_info:
            if relation_info['service_name'] in services_dict:
                service_names.add(relation_info['service_name'])
            else:
                log("Service '%s' does not exist." %
                    relation_info['service_name'])
                continue

        if 'sitenames' in relation_info:
            sitenames = relation_info['sitenames'].split()
            for sitename in sitenames:
                if sitename in services_dict:
                    service_names.add(sitename)

        if juju_service_name + "_service" in services_dict:
            service_names.add(juju_service_name + "_service")

        if juju_service_name in services_dict:
            service_names.add(juju_service_name)

        if not service_names:
            service_names.add(services_dict[None]["service_name"])

        for service_name in service_names:
            service = services_dict[service_name]

            # Add the server entries
            servers = service.setdefault("servers", [])
            servers.append((server_name, host, port,
                            services_dict[service_name].get(
                                'server_options', [])))

    has_servers = False
    for service_name, service in services_dict.iteritems():
        if service.get("servers", []):
            has_servers = True

    if not has_servers:
        log("No backend servers, exiting.")
        return

    del services_dict[None]
    services_dict = ensure_service_host_port(services_dict)
    if config_data["peering_mode"] != "active-active":
        services_dict = apply_peer_config(services_dict)
    write_service_config(services_dict)
    return services_dict


def apply_peer_config(services_dict):
    peer_data = relations_of_type("peer")

    peer_services = {}
    for relation_info in peer_data:
        unit_name = relation_info["__unit__"]
        peer_services_data = relation_info.get("all_services")
        if peer_services_data is None:
            continue
        service_data = yaml.safe_load(peer_services_data)
        for service in service_data:
            service_name = service["service_name"]
            if service_name in services_dict:
                peer_service = peer_services.setdefault(service_name, {})
                peer_service["service_name"] = service_name
                peer_service["service_host"] = service["service_host"]
                peer_service["service_port"] = service["service_port"]
                peer_service["service_options"] = ["balance leastconn",
                                                   "mode tcp",
                                                   "option tcplog"]
                servers = peer_service.setdefault("servers", [])
                servers.append((unit_name.replace("/", "-"),
                                relation_info["private-address"],
                                service["service_port"] + 1, ["check"]))

    if not peer_services:
        return services_dict

    unit_name = os.environ["JUJU_UNIT_NAME"].replace("/", "-")
    private_address = unit_get("private-address")
    for service_name, peer_service in peer_services.iteritems():
        original_service = services_dict[service_name]

        # If the original service has timeout settings, copy them over to the
        # peer service.
        for option in original_service.get("service_options", ()):
            if "timeout" in option:
                peer_service["service_options"].append(option)

        servers = peer_service["servers"]
        # Add ourselves to the list of servers for the peer listen stanza.
        servers.append((unit_name, private_address,
                        original_service["service_port"] + 1,
                        ["check"]))

        # Make all but the first server in the peer listen stanza a backup
        # server.
        servers.sort()
        for server in servers[1:]:
            server[3].append("backup")

        # Remap original service port, will now be used by peer listen stanza.
        original_service["service_port"] += 1

        # Remap original service to a new name, stuff peer listen stanza into
        # it's place.
        be_service = service_name + "_be"
        original_service["service_name"] = be_service
        services_dict[be_service] = original_service
        services_dict[service_name] = peer_service

    return services_dict


def write_service_config(services_dict):
    # Construct the new haproxy.cfg file
    for service_key, service_config in services_dict.items():
        log("Service: %s" % service_key)
        service_name = service_config["service_name"]
        server_entries = service_config.get('servers')
        backends = service_config.get('backends', [])

        errorfiles = service_config.get('errorfiles', [])
        for errorfile in errorfiles:
            path = get_service_lib_path(service_name)
            full_path = os.path.join(
                path, "%s.http" % errorfile["http_status"])
            with open(full_path, 'w') as f:
                f.write(base64.b64decode(errorfile["content"]))

        # Write to disk the content of the given SSL certificates
        crts = service_config.get('crts', [])
        for i, crt in enumerate(crts):
            if crt == "DEFAULT":
                continue
            content = base64.b64decode(crt)
            path = get_service_lib_path(service_name)
            full_path = os.path.join(path, "%d.pem" % i)
            write_ssl_pem(full_path, content)
            with open(full_path, 'w') as f:
                f.write(content)

        if not os.path.exists(default_haproxy_service_config_dir):
            os.mkdir(default_haproxy_service_config_dir, 0o600)
        with open(os.path.join(default_haproxy_service_config_dir,
                               "%s.service" % service_name), 'w') as config:
            config.write(create_listen_stanza(
                service_name,
                service_config['service_host'],
                service_config['service_port'],
                service_config['service_options'],
                server_entries, errorfiles, crts, backends))


def get_service_lib_path(service_name):
    # Get a service-specific lib path
    path = os.path.join(default_haproxy_lib_dir,
                        "service_%s" % service_name)
    if not os.path.exists(path):
        os.makedirs(path)
    return path


# -----------------------------------------------------------------------------
# load_services: Convenience function that loads the service snippet
#                configuration from the filesystem.
# -----------------------------------------------------------------------------
def load_services(service_name=None):
    services = ''
    if service_name is not None:
        if os.path.exists("%s/%s.service" %
                          (default_haproxy_service_config_dir, service_name)):
            with open("%s/%s.service" % (default_haproxy_service_config_dir,
                                         service_name)) as f:
                services = f.read()
        else:
            services = None
    else:
        for service in glob.glob("%s/*.service" %
                                 default_haproxy_service_config_dir):
            with open(service) as f:
                services += f.read()
                services += "\n\n"
    return services


# -----------------------------------------------------------------------------
# remove_services:  Convenience function that removes the configuration
#                   snippets from the filesystem.  This is necessary
#                   To ensure sync between the config/relation-data
#                   and the existing haproxy services.
# -----------------------------------------------------------------------------
def remove_services(service_name=None):
    if service_name is not None:
        path = "%s/%s.service" % (default_haproxy_service_config_dir,
                                  service_name)
        if os.path.exists(path):
            try:
                os.remove(path)
            except Exception as e:
                log(str(e))
                return False
        return True
    else:
        for service in glob.glob("%s/*.service" %
                                 default_haproxy_service_config_dir):
            try:
                os.remove(service)
            except Exception as e:
                log(str(e))
                pass
        return True


# -----------------------------------------------------------------------------
# construct_haproxy_config:  Convenience function to write haproxy.cfg
#                            haproxy_globals, haproxy_defaults,
#                            haproxy_monitoring, haproxy_services
#                            are all strings that will be written without
#                            any checks.
#                            haproxy_monitoring and haproxy_services are
#                            optional arguments
# -----------------------------------------------------------------------------
def construct_haproxy_config(haproxy_globals=None,
                             haproxy_defaults=None,
                             haproxy_monitoring=None,
                             haproxy_services=None):
    if None in (haproxy_globals, haproxy_defaults):
        return
    with open(default_haproxy_config, 'w') as haproxy_config:
        config_string = ''
        for config in (haproxy_globals, haproxy_defaults, haproxy_monitoring,
                       haproxy_services):
            if config is not None:
                config_string += config + '\n\n'
        haproxy_config.write(config_string)


# -----------------------------------------------------------------------------
# service_haproxy:  Convenience function to start/stop/restart/reload
#                   the haproxy service
# -----------------------------------------------------------------------------
def service_haproxy(action=None, haproxy_config=default_haproxy_config):
    if None in (action, haproxy_config):
        return None
    elif action == "check":
        command = ['/usr/sbin/haproxy', '-f', haproxy_config, '-c']
    else:
        command = ['service', 'haproxy', action]
    return_value = subprocess.call(command)
    return return_value == 0


# #############################################################################
# Hook functions
# #############################################################################
def install_hook():
    # Run both during initial install and during upgrade-charm.
    if not os.path.exists(default_haproxy_service_config_dir):
        os.mkdir(default_haproxy_service_config_dir, 0o600)

    config_data = config_get()
    source = config_data.get('source')
    if source == 'backports':
        release = lsb_release()['DISTRIB_CODENAME']
        source = apt_backports_template % {'release': release}
        add_backports_preferences(release)
    add_source(source, config_data.get('key'))
    apt_update(fatal=True)
    apt_install(['haproxy', 'python-jinja2'], fatal=True)
    # Install pyasn1 library and modules for inspecting SSL certificates
    apt_install(['python-pyasn1', 'python-pyasn1-modules'], fatal=False)
    ensure_package_status(service_affecting_packages,
                          config_data['package_status'])
    enable_haproxy()


def config_changed():
    config_data = config_get()

    ensure_package_status(service_affecting_packages,
                          config_data['package_status'])

    old_service_ports = get_service_ports()
    old_stanzas = get_listen_stanzas()
    haproxy_globals = create_haproxy_globals()
    haproxy_defaults = create_haproxy_defaults()
    if config_data['enable_monitoring'] is True:
        haproxy_monitoring = create_monitoring_stanza()
    else:
        haproxy_monitoring = None
    remove_services()
    if config_data.changed("ssl_cert"):
        # TODO: handle also the case where it's the public-address value
        # that changes (see also #1444062)
        _notify_reverseproxy()
    if not create_services():
        sys.exit()
    haproxy_services = load_services()
    update_sysctl(config_data)
    update_ssl_cert(config_data)
    construct_haproxy_config(haproxy_globals,
                             haproxy_defaults,
                             haproxy_monitoring,
                             haproxy_services)

    write_metrics_cronjob(metrics_script_path,
                          metrics_cronjob_path)

    if service_haproxy("check"):
        update_service_ports(old_service_ports, get_service_ports())
        service_haproxy("reload")
        if not (get_listen_stanzas() == old_stanzas):
            notify_website()
            notify_peer()
    else:
        # XXX Ideally the config should be restored to a working state if the
        # check fails, otherwise an inadvertent reload will cause the service
        # to be broken.
        log("HAProxy configuration check failed, exiting.")
        sys.exit(1)
    if config_data.changed("global_log") or config_data.changed("source"):
        # restart rsyslog to pickup haproxy rsyslog config
        # This could be removed once the following bug is fixed in the haproxy
        # package:
        #   https://bugs.debian.org/cgi-bin/bugreport.cgi?bug=790871
        service_restart("rsyslog")


def start_hook():
    if service_haproxy("status"):
        return service_haproxy("restart")
    else:
        return service_haproxy("start")


def stop_hook():
    if service_haproxy("status"):
        return service_haproxy("stop")


def reverseproxy_interface(hook_name=None):
    if hook_name is None:
        return None
    if hook_name == "joined":
        # When we join a new reverseproxy relation we communicate to the
        # remote unit our public IP and public SSL certificate, since
        # some applications might need it in order to tell third parties
        # how to interact with them.
        _notify_reverseproxy(relation_ids=(relation_id(),))
        return
    if hook_name in ("changed", "departed"):
        config_changed()


def _notify_reverseproxy(relation_ids=None):
    config_data = config_get()
    ssl_cert = config_data.get("ssl_cert")
    if ssl_cert == "SELFSIGNED":
        ssl_cert = base64.b64encode(get_selfsigned_cert()[0])
    relation_settings = {
        "public-address": unit_get("public-address"),
        "ssl_cert": ssl_cert,
    }
    for rid in relation_ids or get_relation_ids("reverseproxy"):
        relation_set(relation_id=rid, relation_settings=relation_settings)


def website_interface(hook_name=None):
    if hook_name is None:
        return None
    # Notify website relation but only for the current relation in context.
    notify_website(changed=hook_name == "changed",
                   relation_ids=(relation_id(),))


def get_hostname(host=None):
    my_host = socket.gethostname()
    if host is None or host == "0.0.0.0":
        # If the listen ip has been set to 0.0.0.0 then pass back the hostname
        return socket.getfqdn(my_host)
    elif host == "localhost":
        # If the fqdn lookup has returned localhost (lxc setups) then return
        # hostname
        return my_host
    return host


def notify_relation(relation, changed=False, relation_ids=None):
    default_host = get_hostname()
    default_port = 80

    for rid in relation_ids or get_relation_ids(relation):
        service_names = set()
        if rid is None:
            rid = relation_id()
        for relation_data in relations_for_id(rid):
            if 'service_name' in relation_data:
                service_names.add(relation_data['service_name'])

            if changed:
                if 'is-proxy' in relation_data:
                    remote_service = ("%s__%d" % (relation_data['hostname'],
                                                  relation_data['port']))
                    open("%s/%s.is.proxy" % (
                        default_haproxy_service_config_dir,
                        remote_service), 'a').close()

        service_name = None
        if len(service_names) == 1:
            service_name = service_names.pop()
        elif len(service_names) > 1:
            log("Remote units requested more than a single service name."
                "Falling back to default host/port.")

        if service_name is not None:
            # If a specific service has been asked for then return the ip:port
            # for that service, else pass back the default
            requestedservice = get_config_service(service_name)
            my_host = get_hostname(requestedservice['service_host'])
            my_port = requestedservice['service_port']
        else:
            my_host = default_host
            my_port = default_port

        all_services = ""
        services_dict = create_services()
        if services_dict is not None:
            all_services = yaml.safe_dump(sorted(services_dict.itervalues()))

        relation_set(relation_id=rid, port=str(my_port),
                     hostname=my_host,
                     all_services=all_services)


def notify_website(changed=False, relation_ids=None):
    notify_relation("website", changed=changed, relation_ids=relation_ids)


def notify_peer(changed=False, relation_ids=None):
    notify_relation("peer", changed=changed, relation_ids=relation_ids)


def install_nrpe_scripts():
    scripts_src = os.path.join(os.environ["CHARM_DIR"], "files",
                               "nrpe")
    scripts_dst = "/usr/lib/nagios/plugins"
    if not os.path.exists(scripts_dst):
        os.makedirs(scripts_dst)
    for fname in glob.glob(os.path.join(scripts_src, "*.sh")):
        shutil.copy2(fname,
                     os.path.join(scripts_dst, os.path.basename(fname)))


def update_nrpe_config():
    install_nrpe_scripts()
    nrpe_compat = nrpe.NRPE()
    nrpe_compat.add_check('haproxy', 'Check HAProxy', 'check_haproxy.sh')
    nrpe_compat.add_check('haproxy_queue', 'Check HAProxy queue depth',
                          'check_haproxy_queue_depth.sh')
    nrpe_compat.write()


def delete_metrics_cronjob(cron_path):
    try:
        os.unlink(cron_path)
    except OSError:
        pass


def write_metrics_cronjob(script_path, cron_path):
    config_data = config_get()

    if config_data['enable_monitoring'] is False:
        log("enable_monitoring must be set to true for metrics")
        delete_metrics_cronjob(cron_path)
        return

    # need the following two configs to be valid
    metrics_target = config_data['metrics_target'].strip()
    metrics_sample_interval = config_data['metrics_sample_interval']
    if (not metrics_target or
            ':' not in metrics_target or not
            metrics_sample_interval):
        log("Required config not found or invalid "
            "(metrics_target, metrics_sample_interval), "
            "disabling metrics")
        delete_metrics_cronjob(cron_path)
        return

    charm_dir = os.environ['CHARM_DIR']
    statsd_host, statsd_port = metrics_target.split(':', 1)
    metrics_prefix = config_data['metrics_prefix'].strip()
    metrics_prefix = metrics_prefix.replace(
        "$UNIT", local_unit().replace('.', '-').replace('/', '-'))
    haproxy_hostport = ":".join(['localhost',
                                str(config_data['monitoring_port'])])
    haproxy_httpauth = ":".join([config_data['monitoring_username'].strip(),
                                get_monitoring_password()])

    # ensure script installed
    shutil.copy2('%s/files/metrics/haproxy_to_statsd.sh' % charm_dir,
                 metrics_script_path)

    # write the crontab
    with open(cron_path, 'w') as cronjob:
        cronjob.write(render_template("metrics_cronjob.template", {
            'interval': config_data['metrics_sample_interval'],
            'script': script_path,
            'metrics_prefix': metrics_prefix,
            'metrics_sample_interval': metrics_sample_interval,
            'haproxy_hostport': haproxy_hostport,
            'haproxy_httpauth': haproxy_httpauth,
            'statsd_host': statsd_host,
            'statsd_port': statsd_port,
        }))


def add_backports_preferences(release):
    with open(haproxy_preferences_path, "w") as preferences:
        preferences.write(
            "Package: haproxy\n"
            "Pin: release a=%(release)s-backports\n"
            "Pin-Priority: 500\n" % {'release': release})


def has_ssl_support():
    """Return True if the locally installed haproxy package supports SSL."""
    cache = apt_cache()
    package = cache["haproxy"]
    return package.current_ver.ver_str.split(".")[0:2] >= ["1", "5"]


def get_selfsigned_cert():
    """Return the content of the self-signed certificate.

    If no self-signed certificate is there or the existing one doesn't match
    our unit data, a new one will be created.

    @return: A 2-tuple whose first item holds the content of the public
        certificate and the second item the content of the private key.
    """
    cert_file = os.path.join(default_haproxy_lib_dir, "selfsigned_ca.crt")
    key_file = os.path.join(default_haproxy_lib_dir, "selfsigned.key")
    if is_selfsigned_cert_stale(cert_file, key_file):
        log("Generating self-signed certificate")
        gen_selfsigned_cert(cert_file, key_file)
    result = ()
    for content_file in [cert_file, key_file]:
        with open(content_file, "r") as fd:
            result += (fd.read(),)
    return result


# XXX taken from the apache2 charm.
def is_selfsigned_cert_stale(cert_file, key_file):
    """
    Do we need to generate a new self-signed cert?

    @param cert_file: destination path of generated certificate
    @param key_file: destination path of generated private key
    """
    # Basic Existence Checks
    if not os.path.exists(cert_file):
        return True
    if not os.path.exists(key_file):
        return True

    # Common Name
    from OpenSSL import crypto
    with open(cert_file) as fd:
        cert = crypto.load_certificate(
            crypto.FILETYPE_PEM, fd.read())
    cn = cert.get_subject().commonName
    if unit_get('public-address') != cn:
        return True

    # Subject Alternate Name -- only trusty+ support this
    try:
        from pyasn1.codec.der import decoder
        from pyasn1_modules import rfc2459
    except ImportError:
        log('Cannot check subjAltName on <= 12.04, skipping.')
        return False
    cert_addresses = set()
    unit_addresses = set(
        [unit_get('public-address'), unit_get('private-address')])
    for i in range(0, cert.get_extension_count()):
        extension = cert.get_extension(i)
        try:
            names = decoder.decode(
                extension.get_data(), asn1Spec=rfc2459.SubjectAltName())[0]
            for name in names:
                cert_addresses.add(str(name.getComponent()))
        except Exception:
            pass
    if cert_addresses != unit_addresses:
        log('subjAltName: Cert (%s) != Unit (%s), assuming stale' % (
            cert_addresses, unit_addresses))
        return True

    return False


# XXX taken from the apache2 charm.
def gen_selfsigned_cert(cert_file, key_file):
    """
    Create a self-signed certificate.

    @param cert_file: destination path of generated certificate
    @param key_file: destination path of generated private key
    """
    os.environ['OPENSSL_CN'] = unit_get('public-address')
    os.environ['OPENSSL_PUBLIC'] = unit_get("public-address")
    os.environ['OPENSSL_PRIVATE'] = unit_get("private-address")
    # Set the umask so the child process will inherit it and
    # the generated files will be readable only by root..
    old_mask = os.umask(0o77)
    subprocess.call(
        ['openssl', 'req', '-new', '-x509', '-nodes', '-config',
         os.path.join(os.environ['CHARM_DIR'], 'data', 'openssl.cnf'),
         '-keyout', key_file, '-out', cert_file, '-days', '3650'],)
    os.umask(old_mask)
    uid = pwd.getpwnam('haproxy').pw_uid
    os.chown(key_file, uid, -1)
    os.chown(cert_file, uid, -1)


def write_ssl_pem(path, content):
    """Write an SSL pem file and set permissions on it."""
    # Set the umask so the child process will inherit it and we
    # can make certificate files readable only by the 'haproxy'
    # user (see below).
    old_mask = os.umask(0o77)
    with open(path, 'w') as f:
        f.write(content)
    os.umask(old_mask)
    uid = pwd.getpwnam('haproxy').pw_uid
    os.chown(path, uid, -1)


def statistics_interface():
    config = config_get()
    enable_monitoring = config['enable_monitoring']
    monitoring_port = config['monitoring_port']
    monitoring_password = get_monitoring_password()
    monitoring_username = config['monitoring_username']
    for relid in get_relation_ids('statistics'):
        if not enable_monitoring:
            relation_set(relation_id=relid,
                         enabled=enable_monitoring)
        else:
            relation_set(relation_id=relid,
                         enabled=enable_monitoring,
                         port=monitoring_port,
                         password=monitoring_password,
                         user=monitoring_username)


# #############################################################################
# Main section
# #############################################################################


def main(hook_name):
    if hook_name == "install":
        install_hook()
    elif hook_name == "upgrade-charm":
        install_hook()
        config_changed()
        update_nrpe_config()
    elif hook_name == "config-changed":
        config_data = config_get()
        if config_data.changed("source"):
            install_hook()
        config_changed()
        update_nrpe_config()
        statistics_interface()
        if config_data.implicit_save:
            config_data.save()
    elif hook_name == "start":
        start_hook()
    elif hook_name == "stop":
        stop_hook()
    elif hook_name == "reverseproxy-relation-broken":
        config_changed()
    elif hook_name == "reverseproxy-relation-changed":
        reverseproxy_interface("changed")
    elif hook_name == "reverseproxy-relation-departed":
        reverseproxy_interface("departed")
    elif hook_name == "reverseproxy-relation-joined":
        reverseproxy_interface("joined")
    elif hook_name == "website-relation-joined":
        website_interface("joined")
    elif hook_name == "website-relation-changed":
        website_interface("changed")
    elif hook_name == "peer-relation-joined":
        website_interface("joined")
    elif hook_name == "peer-relation-changed":
        reverseproxy_interface("changed")
    elif hook_name in ("nrpe-external-master-relation-joined",
                       "local-monitors-relation-joined"):
        update_nrpe_config()
    elif hook_name in ("statistics-relation-joined",
                       "statistics-relation-changed"):
        statistics_interface()
    else:
        print("Unknown hook")
        sys.exit(1)


if __name__ == "__main__":
    hook_name = os.path.basename(sys.argv[0])
    # Also support being invoked directly with hook as argument name.
    if hook_name == "hooks.py":
        if len(sys.argv) < 2:
            sys.exit("Missing required hook name argument.")
        hook_name = sys.argv[1]
    main(hook_name)
