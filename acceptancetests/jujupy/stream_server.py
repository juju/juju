# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2018 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# Provides functionality for running a local streams server.

from __future__ import print_function

try:
    from BaseHTTPServer import HTTPServer
except ImportError:
    from http.server import HTTPServer

try:
    from SimpleHTTPServer import SimpleHTTPRequestHandler
except ImportError:
    from http.server import SimpleHTTPRequestHandler

from datetime import datetime
import multiprocessing
import hashlib
import logging
import os
import shutil
import socket
import subprocess
import tarfile

from contextlib import contextmanager

from jujupy.client import (
    get_version_string_parts
)

__metaclass__ = type


log = logging.getLogger(__name__)


class _JujuStreamData:
    """Models stream metadata. Best used via StreamServer."""

    def __init__(self, working_dir):
        """Models Juju product simplestream metadata.

        :param working_dir: Directory in which to copy agent tarballs and
          generate stream json.
        """
        self.products = []

        self._agent_path = os.path.join(working_dir, 'agent')
        self._stream_path = working_dir

        os.makedirs(self._agent_path)

    def add_product(self, content_id, version, arch, agent_tgz_path):
        """Add a new product to generate stream data for.

        :param content_id: String ID (e.g.'proposed', 'release')
        :param version: Juju version string for product (i.e. '2.3.3')
        :param arch: Architecture string of this product (e.g. 's390x','amd64')
        :param agent_tgz_path: String full path to agent tarball file to use.
          This file is copied into the JujuStreamData working dir to be served
          up at a later date.
        """
        shutil.copy(agent_tgz_path, self._agent_path)
        product_dict = _generate_product_json(
            content_id, version, arch, agent_tgz_path)
        self.products.append(product_dict)

    def generate_stream_data(self):
        """Generate metadata from added products into working dir."""
        # Late import as simplestreams.log overwrites logging handlers.
        from simplestreams.json2streams import (
            dict_to_item,
            write_juju_streams
        )
        from simplestreams.generate_simplestreams import items2content_trees
        # The following has been cribbed from simplestreams.json2streams.
        # Doing so saves the need to create json files to then shell out to
        # read those files into memory to generate the resulting json files.
        items = (dict_to_item(item.copy()) for item in self.products)
        updated = datetime.utcnow().strftime(
            '%a, %d %b %Y %H:%M:%S +0000')
        data = {'updated': updated, 'datatype': 'content-download'}
        trees = items2content_trees(items, data)
        return write_juju_streams(self._stream_path, trees, updated)


class StreamServer:
    """Provide service to create stream metadata and to serve it."""

    def __init__(self, base_dir, stream_data_type=_JujuStreamData):
        self.base_dir = base_dir
        self.stream_data = stream_data_type(base_dir)

    def add_product(self, content_id, version, arch, agent_tgz_path):
        """Add a new product to generate stream data for.

        :param content_id: String ID (e.g.'proposed', 'released')
        :param version: Juju version string for product (i.e. '2.3.3')
        :param arch: Architecture string of this product (e.g. 's390x','amd64')
        :param agent_tgz_path: String full path to agent tarball file to use.
          This file is copied into the JujuStreamData working dir to be served
          up at a later date.
        """
        self.stream_data.add_product(
            content_id, version, arch, agent_tgz_path)
        # Re-generate when adding a product allows updating the server while
        # running.
        # Can be noisey in the logs, if a lot of products need to be added can
        # use StreamServer.stream_data.add_product() directly.
        self.stream_data.generate_stream_data()

    @contextmanager
    def server(self):
        """Serves the products that have been added up until this point.

        :yields: The http address of the server, including the port used.
        """
        self.stream_data.generate_stream_data()
        server = _create_stream_server()
        ip_address, port = _get_server_address(server)
        address = 'http://{}:{}'.format(ip_address, port)
        server_process = multiprocessing.Process(
            target=_http_worker,
            args=(server, self.base_dir),
            name='SimlestreamServer')
        try:
            log.info('Starting stream server at: {}'.format(address))
            multiprocessing.log_to_stderr(logging.DEBUG)
            server_process.start()
            yield address
        finally:
            log.info('Terminating stream server')
            server_process.terminate()
            server_process.join()


def agent_tgz_from_juju_binary(
        juju_bin_path, tmp_dir, series=None, force_version=None):
    """
    Create agent tarball with jujud found with provided juju binary.

    Search the location where `juju_bin_path` resides to attempt to find a
    jujud in the same location.

    :param juju_bin_path: The path to the juju bin in use.
    :param tmp_dir: Location to store the generated agent file.
    :param series: String series to use instead of that of the passed binary.
      Allows one to overwrite the series of the juju client.
    :returns: String path to generated
    """
    def _series_lookup(series):
        # Handle the inconsistencies with agent series names.
        if series is None:
            return None
        if series.startswith('centos'):
            return series
        if series.startswith('win'):
            return 'win2012'
        return 'ubuntu'

    bin_dir = os.path.dirname(juju_bin_path)
    try:
        jujud_path = os.path.join(
            bin_dir,
            [f for f in os.listdir(bin_dir) if f == 'jujud'][0])
    except IndexError:
        raise RuntimeError('Unable to find jujud binary in {}'.format(bin_dir))

    try:
        version_output = subprocess.check_output(
            [jujud_path, 'version']).rstrip(str.encode('\n'))
        version, bin_series, arch = get_version_string_parts(version_output)
        bin_agent_series = _series_lookup(bin_series)
    except subprocess.CalledProcessError as e:
        raise RuntimeError(
            'Unable to query jujud for version details: {}'.format(e))
    except IndexError:
        raise RuntimeError(
            'Unable to determine version, series and arch from version '
            'string: {}'.format(version_output))

    version = force_version or version
    agent_tgz_name = 'juju-{version}-{series}-{arch}.tgz'.format(
        version=version,
        series=series if series else bin_agent_series,
        arch=arch
    )

    # It's possible we're re-generating a file.
    tgz_path = os.path.join(tmp_dir, agent_tgz_name)
    if os.path.exists(tgz_path):
        log.debug('Reusing agent file: {}'.format(agent_tgz_name))
        return tgz_path

    log.debug('Creating agent file: {}'.format(agent_tgz_name))
    with tarfile.open(tgz_path, 'w:gz') as tar:
        tar.add(jujud_path, arcname='jujud')
        if force_version is not None:
            force_version_file = os.path.join(tmp_dir, 'FORCE-VERSION')
            with open(force_version_file, 'wt') as f:
                f.write(version)
            tar.add(force_version_file, arcname='FORCE-VERSION')
    return tgz_path


def _generate_product_json(content_id, version, arch, series, agent_tgz_path):
    """Return dict containing product metadata from provided args."""
    tgz_name = os.path.basename(agent_tgz_path)
    file_details = _get_tgz_file_details(agent_tgz_path)
    item_name = '{version}-ubuntu-{arch}'.format(
        version=version,
        arch=arch)
    return dict(
        arch=arch,
        content_id='com.ubuntu.juju:{}:agents'.format(content_id),
        format='products:1.0',
        ftype='tar.gz',
        item_name=item_name,
        md5=file_details['md5'],
        path=os.path.join('agent', tgz_name),
        product_name='com.ubuntu.juju:ubuntu:{arch}'.format(
            arch=arch),
        release='ubuntu',
        sha256=file_details['sha256'],
        size=file_details['size'],
        version=version,
        version_name=datetime.utcnow().strftime('%Y%m%d')
    )


def _get_series_details(series):
    # Ubuntu agents use series and a code (i.e. trusty:14.04), others don't.
    _series_lookup = dict(
        trusty=14.04,
        xenial=16.04,
        artful=17.10,
        bionic=18.04,
    )
    try:
        series_code = _series_lookup[series]
    except KeyError:
        return series, series
    return series, series_code


def _get_tgz_file_details(agent_tgz_path):
    file_details = dict(size=os.path.getsize(agent_tgz_path))
    with open(agent_tgz_path, 'rb') as f:
        content = f.read()
    for hashtype in 'md5', 'sha256':
        hash_obj = hashlib.new(hashtype)
        hash_obj.update(content)
        file_details[hashtype] = hash_obj.hexdigest()

    return file_details


def _get_server_address(httpd_server):
    # Attempt to get the "primary" IP from this machine to provide an
    # addressable IP (not 0.0.0.0 or localhost etc.)
    # Taken from:
    # https://stackoverflow.com/questions/166506/finding-local-ip-addresses-using-pythons-stdlib
    s = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    try:
        # Just any ol' non-routable address.
        s.connect(('10.255.255.255', 0))
        return s.getsockname()[0], httpd_server.server_port
    except:
        raise RuntimeError('Unable to serve on an addressable IP.')
    finally:
        s.close()


class _QuietHttpRequestHandler(SimpleHTTPRequestHandler):

    def log_message(self, format, *args):
        # Lessen the output
        log.debug('{} - - [{}] {}'.format(
            self.client_address[0],
            self.client_address[0],
            format % args))


def _create_stream_server():
    server_details = ("", 0)
    httpd = HTTPServer(server_details, _QuietHttpRequestHandler)
    return httpd


def _http_worker(httpd, serve_base):
    """Serve `serve_base` dir using `httpd` SocketServer.TCPServer object."""
    log.debug('Starting server with root: "{}"'.format(serve_base))
    try:
        os.chdir(serve_base)
        httpd.serve_forever()
    except Exception as e:
        print('Exiting due to exception: {}'.format(e))
