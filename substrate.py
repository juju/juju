from contextlib import (
    contextmanager,
)
import json
import logging
import os
import subprocess
from time import sleep
import urlparse

from boto import ec2
from boto.exception import EC2ResponseError

import get_ami
from jujuconfig import (
    get_euca_env,
    translate_to_env,
)
from utility import (
    print_now,
    temp_dir,
    until_timeout,
)


__metaclass__ = type


LIBVIRT_DOMAIN_RUNNING = 'running'
LIBVIRT_DOMAIN_SHUT_OFF = 'shut off'


class StillProvisioning(Exception):
    """Attempted to terminate instances still provisioning."""

    def __init__(self, instance_ids):
        super(StillProvisioning, self).__init__(
            'Still provisioning: {}'.format(', '.join(instance_ids)))
        self.instance_ids = instance_ids


def terminate_instances(env, instance_ids):
    if len(instance_ids) == 0:
        print_now("No instances to delete.")
        return
    provider_type = env.config.get('type')
    environ = dict(os.environ)
    if provider_type == 'ec2':
        environ.update(get_euca_env(env.config))
        command_args = ['euca-terminate-instances'] + instance_ids
    elif provider_type in ('openstack', 'rackspace'):
        environ.update(translate_to_env(env.config))
        command_args = ['nova', 'delete'] + instance_ids
    elif provider_type == 'maas':
        with MAASAccount.manager_from_config(env.config) as substrate:
            substrate.terminate_instances(instance_ids)
        return
    elif provider_type == 'lxd':
        with LXDAccount.manager_from_config(env.config) as substrate:
            substrate.terminate_instances(instance_ids)
        return
    else:
        with make_substrate_manager(env.config) as substrate:
            if substrate is None:
                raise ValueError(
                    "This test does not support the %s provider"
                    % provider_type)
            return substrate.terminate_instances(instance_ids)
    print_now("Deleting %s." % ', '.join(instance_ids))
    subprocess.check_call(command_args, env=environ)


class AWSAccount:
    """Represent the credentials of an AWS account."""

    @classmethod
    @contextmanager
    def manager_from_config(cls, config, region=None):
        """Create an AWSAccount from a juju environment dict."""
        euca_environ = get_euca_env(config)
        if region is None:
            region = config["region"]
        client = ec2.connect_to_region(
            region, aws_access_key_id=euca_environ['EC2_ACCESS_KEY'],
            aws_secret_access_key=euca_environ['EC2_SECRET_KEY'])
        yield cls(euca_environ, region, client)

    def __init__(self, euca_environ, region, client):
        self.euca_environ = euca_environ
        self.region = region
        self.client = client

    def iter_security_groups(self):
        """Iterate through security groups created by juju in this account.

        :return: an iterator of (group-id, group-name) tuples.
        """
        groups = self.client.get_all_security_groups(
            filters={'description': 'juju group'})
        for group in groups:
            yield group.id, group.name

    def iter_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account.

        :param instance_ids: If supplied, list only security groups used by
            the specified instances.
        :return: an iterator of (group-id, group-name) tuples.
        """
        logging.info('Listing security groups in use.')
        reservations = self.client.get_all_instances(instance_ids=instance_ids)
        for reservation in reservations:
            for instance in reservation.instances:
                for group in instance.groups:
                    yield group.id, group.name

    def destroy_security_groups(self, groups):
        """Destroy the specified security groups.

        :return: a list of groups that could not be destroyed.
        """
        failures = []
        for group in groups:
            deleted = self.client.delete_security_group(name=group)
            if not deleted:
                failures.append(group)
        return failures

    def delete_detached_interfaces(self, security_groups):
        """Delete detached network interfaces for supplied groups.

        :param security_groups: A collection of security_group ids.
        :return: A collection of security groups which still have interfaces in
            them.
        """
        interfaces = self.client.get_all_network_interfaces(
            filters={'status': 'available'})
        unclean = set()
        for interface in interfaces:
            for group in interface.groups:
                if group.id in security_groups:
                    try:
                        interface.delete()
                    except EC2ResponseError as e:
                        if e.error_code not in (
                                'InvalidNetworkInterface.InUse',
                                'InvalidNetworkInterfaceID.NotFound'):
                            raise
                        logging.info(
                            'Failed to delete interface {!r}. {}'.format(
                                interface.id, e.message))
                        unclean.update(g.id for g in interface.groups)
                    break
        return unclean


class OpenStackAccount:
    """Represent the credentials/region of an OpenStack account."""

    def __init__(self, username, password, tenant_name, auth_url, region_name):
        self._username = username
        self._password = password
        self._tenant_name = tenant_name
        self._auth_url = auth_url
        self._region_name = region_name
        self._client = None

    @classmethod
    @contextmanager
    def manager_from_config(cls, config):
        """Create an OpenStackAccount from a juju environment dict."""
        yield cls(
            config['username'], config['password'], config['tenant-name'],
            config['auth-url'], config['region'])

    def get_client(self):
        """Return a novaclient Client for this account."""
        from novaclient import client
        return client.Client(
            '1.1', self._username, self._password, self._tenant_name,
            self._auth_url, region_name=self._region_name,
            service_type='compute', insecure=False)

    @property
    def client(self):
        """A novaclient Client for this account.  May come from cache."""
        if self._client is None:
            self._client = self.get_client()
        return self._client

    def iter_security_groups(self):
        """Iterate through security groups created by juju in this account.

        :return: an iterator of (group-id, group-name) tuples.
        """
        return ((g.id, g.name) for g in self.client.security_groups.list()
                if g.description == 'juju group')

    def iter_instance_security_groups(self, instance_ids=None):
        """List the security groups used by instances in this account.

        :param instance_ids: If supplied, list only security groups used by
            the specified instances.
        :return: an iterator of (group-id, group-name) tuples.
        """
        group_names = set()
        for server in self.client.servers.list():
            if instance_ids is not None and server.id not in instance_ids:
                continue
            # A server that errors before security groups are assigned will
            # have no security_groups attribute.
            groups = (getattr(server, 'security_groups', []))
            group_names.update(group['name'] for group in groups)
        return ((k, v) for k, v in self.iter_security_groups()
                if v in group_names)


class JoyentAccount:
    """Represent a Joyent account."""

    def __init__(self, client):
        self.client = client

    @classmethod
    @contextmanager
    def manager_from_config(cls, config):
        """Create a ContextManager for a JoyentAccount.

         Using a juju environment dict, the private key is written to a
         tmp file. Then, the Joyent client is inited with the path to the
         tmp key. The key is removed when done.
         """
        from joyent import Client
        with temp_dir() as key_dir:
            key_path = os.path.join(key_dir, 'joyent.key')
            open(key_path, 'w').write(config['private-key'])
            client = Client(
                config['sdc-url'], config['manta-user'],
                config['manta-key-id'], key_path, '')
            yield cls(client)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        provisioning = []
        for instance_id in instance_ids:
            machine_info = self.client._list_machines(instance_id)
            if machine_info['state'] == 'provisioning':
                provisioning.append(instance_id)
                continue
            self._terminate_instance(instance_id)
        if len(provisioning) > 0:
            raise StillProvisioning(provisioning)

    def _terminate_instance(self, machine_id):
        logging.info('Stopping instance {}'.format(machine_id))
        self.client.stop_machine(machine_id)
        for ignored in until_timeout(30):
            stopping_machine = self.client._list_machines(machine_id)
            if stopping_machine['state'] == 'stopped':
                break
            sleep(3)
        else:
            raise Exception('Instance did not stop: {}'.format(machine_id))
        logging.info('Terminating instance {}'.format(machine_id))
        self.client.delete_machine(machine_id)


class AzureAccount:
    """Represent an Azure Account."""

    def __init__(self, service_client):
        """Constructor.

        :param service_client: An instance of
            azure.servicemanagement.ServiceManagementService.
        """
        self.service_client = service_client

    @classmethod
    @contextmanager
    def manager_from_config(cls, config):
        """A context manager for a AzureAccount.

        It writes the certificate to a temp file because the Azure client
        library requires it, then deletes the temp file when done.
        """
        from azure.servicemanagement import ServiceManagementService
        with temp_dir() as cert_dir:
            cert_file = os.path.join(cert_dir, 'azure.pem')
            open(cert_file, 'w').write(config['management-certificate'])
            service_client = ServiceManagementService(
                config['management-subscription-id'], cert_file)
            yield cls(service_client)

    @staticmethod
    def convert_instance_ids(instance_ids):
        """Convert juju instance ids into Azure service/role names.

        Return a dict mapping service name to role names.
        """
        services = {}
        for instance_id in instance_ids:
            service, role = instance_id.rsplit('-', 1)
            services.setdefault(service, set()).add(role)
        return services

    @contextmanager
    def terminate_instances_cxt(self, instance_ids):
        """Terminate instances in a context.

        This context manager requests termination, then allows the "with"
        block to happen.  When the block is exited, it waits until the
        operations complete.

        The strategy for terminating instances varies depending on whether all
        roles are being terminated.  If all roles are being terminated, the
        deployment and hosted service are deleted.  If not all roles are being
        terminated, the roles themselves are deleted.
        """
        converted = self.convert_instance_ids(instance_ids)
        requests = set()
        services_to_delete = set(converted.keys())
        for service, roles in converted.items():
            properties = self.service_client.get_hosted_service_properties(
                service, embed_detail=True)
            for deployment in properties.deployments:
                role_names = set(
                    d_role.role_name for d_role in deployment.role_list)
                if role_names.difference(roles) == set():
                    requests.add(self.service_client.delete_deployment(
                        service, deployment.name))
                else:
                    services_to_delete.discard(service)
                    for role in roles:
                        requests.add(
                            self.service_client.delete_role(
                                service, deployment.name, role))
        yield
        self.block_on_requests(requests)
        for service in services_to_delete:
            self.service_client.delete_hosted_service(service)

    def block_on_requests(self, requests):
        """Wait until the requests complete."""
        requests = set(requests)
        while len(requests) > 0:
            for request in list(requests):
                op = self.service_client.get_operation_status(
                    request.request_id)
                if op.status == 'Succeeded':
                    requests.remove(request)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances.

        See terminate_instances_cxt for details.
        """
        with self.terminate_instances_cxt(instance_ids):
            return


class MAASAccount:
    """Represent a Mass account."""

    def __init__(self, profile, url, oauth):
        self.profile = profile
        self.url = urlparse.urljoin(url, 'api/1.0/')
        self.oauth = oauth

    @classmethod
    @contextmanager
    def manager_from_config(cls, config):
        """Create a ContextManager for a MaasAccount."""
        manager = cls(
            config['name'], config['maas-server'], config['maas-oauth'])
        manager.login()
        yield manager
        manager.logout()

    def login(self):
        """Login with the maas cli."""
        subprocess.check_call(
            ['maas', 'login', self.profile, self.url, self.oauth])

    def logout(self):
        """Logout with the maas cli."""
        subprocess.check_call(
            ['maas', 'logout', self.profile])

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        for instance in instance_ids:
            maas_system_id = instance.split('/')[5]
            print_now('Deleting %s.' % instance)
            subprocess.check_call(
                ['maas', self.profile, 'node', 'release', maas_system_id])

    def get_allocated_nodes(self):
        """Return a dict of allocated nodes with the hostname as keys."""
        data = subprocess.check_output(
            ['maas', self.profile, 'nodes', 'list-allocated'])
        nodes = json.loads(data)
        allocated = {node['hostname']: node for node in nodes}
        return allocated

    def get_allocated_ips(self):
        """Return a dict of allocated ips with the hostname as keys.

        A maas node may have many ips. The method selects the first ip which
        is the address used for virsh access and ssh.
        """
        allocated = self.get_allocated_nodes()
        ips = {k: v['ip_addresses'][0] for k, v in allocated.items()
               if v['ip_addresses']}
        return ips


class LXDAccount:
    """Represent a LXD account."""

    def __init__(self, remote=None):
        self.remote = remote

    @classmethod
    @contextmanager
    def manager_from_config(cls, config):
        """Create a ContextManager for a LXDAccount."""
        remote = config.get('region', None)
        yield cls(remote=remote)

    def terminate_instances(self, instance_ids):
        """Terminate the specified instances."""
        for instance_id in instance_ids:
            subprocess.check_call(['lxc', 'stop', '--force', instance_id])
            if self.remote:
                instance_id = '{}:{}'.format(self.remote, instance_id)
            subprocess.check_call(['lxc', 'delete', '--force', instance_id])


@contextmanager
def make_substrate_manager(config):
    """A ContextManager that returns an Account for the config's substrate.

    Returns None if the substrate is not supported.
    """
    substrate_factory = {
        'ec2': AWSAccount.manager_from_config,
        'openstack': OpenStackAccount.manager_from_config,
        'rackspace': OpenStackAccount.manager_from_config,
        'joyent': JoyentAccount.manager_from_config,
        'azure': AzureAccount.manager_from_config,
        'lxd': LXDAccount.manager_from_config,
    }
    factory = substrate_factory.get(config['type'])
    if factory is None:
        yield None
    else:
        with factory(config) as substrate:
            yield substrate


def start_libvirt_domain(uri, domain):
    """Call virsh to start the domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', uri, 'start', domain]
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        if 'already active' in e.output:
            return '%s is already running; nothing to do.' % domain
        raise Exception('%s failed:\n %s' % (command, e.output))
    sleep(30)
    for ignored in until_timeout(120):
        if verify_libvirt_domain(uri, domain, LIBVIRT_DOMAIN_RUNNING):
            return "%s is now running" % domain
        sleep(2)
    raise Exception('libvirt domain %s did not start.' % domain)


def stop_libvirt_domain(uri, domain):
    """Call virsh to shutdown the domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', uri, 'shutdown', domain]
    try:
        subprocess.check_output(command, stderr=subprocess.STDOUT)
    except subprocess.CalledProcessError as e:
        if 'domain is not running' in e.output:
            return ('%s is not running; nothing to do.' % domain)
        raise Exception('%s failed:\n %s' % (command, e.output))
    sleep(30)
    for ignored in until_timeout(120):
        if verify_libvirt_domain(uri, domain, LIBVIRT_DOMAIN_SHUT_OFF):
            return "%s is now shut off" % domain
        sleep(2)
    raise Exception('libvirt domain %s is not shut off.' % domain)


def verify_libvirt_domain(uri, domain, state=LIBVIRT_DOMAIN_RUNNING):
    """Returns a bool based on if the domain is in the given state.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    @Parm state: The state to verify (e.g. "running or "shut off").
    """

    dom_status = get_libvirt_domstate(uri, domain)
    return state in dom_status


def get_libvirt_domstate(uri, domain):
    """Call virsh to get the state of the given domain.

    @Parms URI: The address of the libvirt service.
    @Parm domain: The name of the domain.
    """

    command = ['virsh', '-c', uri, 'domstate', domain]
    try:
        sub_output = subprocess.check_output(command)
    except subprocess.CalledProcessError:
        raise Exception('%s failed' % command)
    return sub_output


def parse_euca(euca_output):
    for line in euca_output.splitlines():
        fields = line.split('\t')
        if fields[0] != 'INSTANCE':
            continue
        yield fields[1], fields[3]


def run_instances(count, job_name, series, region=None):
    """create a number of instances in ec2 and tag them.

    :param count: The number of instances to create.
    :param job_name: The name of job that owns the instances (used as a tag).
    :param series: The series to run in the instance.
        If None, Precise will be used.
    """
    if series is None:
        series = 'precise'
    environ = dict(os.environ)
    ami = get_ami.query_ami(series, "amd64", region=region)
    command = [
        'euca-run-instances', '-k', 'id_rsa', '-n', '%d' % count,
        '-t', 'm1.large', '-g', 'manual-juju-test', ami]
    run_output = subprocess.check_output(command, env=environ).strip()
    machine_ids = dict(parse_euca(run_output)).keys()
    for remaining in until_timeout(300):
        try:
            names = dict(describe_instances(machine_ids, env=environ))
            if '' not in names.values():
                subprocess.check_call(
                    ['euca-create-tags', '--tag', 'job_name=%s' % job_name] +
                    machine_ids, env=environ)
                return names.items()
        except subprocess.CalledProcessError:
            subprocess.call(['euca-terminate-instances'] + machine_ids)
            raise
        sleep(1)


def describe_instances(instances=None, running=False, job_name=None,
                       env=None):
    command = ['euca-describe-instances']
    if job_name is not None:
        command.extend(['--filter', 'tag:job_name=%s' % job_name])
    if running:
        command.extend(['--filter', 'instance-state-name=running'])
    if instances is not None:
        command.extend(instances)
    logging.info(' '.join(command))
    return parse_euca(subprocess.check_output(command, env=env))


def get_job_instances(job_name):
    description = describe_instances(job_name=job_name, running=True)
    return (machine_id for machine_id, name in description)


def destroy_job_instances(job_name):
    instances = list(get_job_instances(job_name))
    if len(instances) == 0:
        return
    subprocess.check_call(['euca-terminate-instances'] + instances)


def resolve_remote_dns_names(env, remote_machines):
    """Update addresses of given remote_machines as needed by providers."""
    if env.config['type'] != 'maas':
        # Only MAAS requires special handling at prsent.
        return
    # MAAS hostnames are not resolvable, but we can adapt them to IPs.
    with MAASAccount.manager_from_config(env.config) as account:
        allocated_ips = account.get_allocated_ips()
    for remote in remote_machines:
        if remote.get_address() in allocated_ips:
            remote.update_address(allocated_ips[remote.address])
