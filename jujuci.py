#!/usr/bin/python
"""Access Juju CI artifacts and data."""

from __future__ import print_function

from argparse import ArgumentParser
import base64
from collections import namedtuple
import fnmatch
import json
import os
import shutil
import sys
import traceback
import urllib
import urllib2

from deploy_stack import destroy_environment
from jujuconfig import NoSuchEnvironment
from jujupy import (
    client_from_config,
)
try:
    from lsb_release import get_distro_information
except ImportError:
    def get_distro_information():
        raise NotImplementedError('Not supported on this platform!')

from utility import (
    extract_deb,
    get_deb_arch,
    get_revision_build,
    print_now,
    )


__metaclass__ = type


JENKINS_URL = 'http://juju-ci.vapour.ws:8080'
BUILD_REVISION = 'build-revision'
PUBLISH_REVISION = 'publish-revision'
CERTIFY_UBUNTU_PACKAGES = 'certify-ubuntu-packages'

Artifact = namedtuple('Artifact', ['file_name', 'location'])


Credentials = namedtuple('Credentials', ['user', 'password'])


class CredentialsMissing(Exception):
    """Raised when no credentials are supplied."""


def get_jenkins_json(credentials, url):
    req = urllib2.Request(url)
    encoded = base64.encodestring(
        '{}:{}'.format(*credentials)).replace('\n', '')
    req.add_header('Authorization', 'Basic {}'.format(encoded))
    build_data = urllib2.urlopen(req)
    return json.load(build_data)


def get_build_data(jenkins_url, credentials, job_name,
                   build='lastSuccessfulBuild'):
    """Return a dict of the build data for a job build number."""
    url = '%s/job/%s/%s/api/json' % (jenkins_url, job_name, build)
    return get_jenkins_json(credentials, url)


def get_job_data(jenkins_url, credentials, job_name):
    """Return a dict of the job data for a job name."""
    url = '%s/job/%s/api/json' % (jenkins_url, job_name)
    return get_jenkins_json(credentials, url)


def make_artifact(build_data, artifact):
    location = '%sartifact/%s' % (build_data['url'], artifact['relativePath'])
    return Artifact(artifact['fileName'], location)


def find_artifacts(build_data, glob='*'):
    found = []
    for artifact in build_data['artifacts']:
        file_name = artifact['fileName']
        if fnmatch.fnmatch(file_name, glob):
            found.append(make_artifact(build_data, artifact))
    return found


def list_artifacts(credentials, job_name, build, glob, verbose=False):
    build_data = get_build_data(JENKINS_URL, credentials, job_name, build)
    artifacts = find_artifacts(build_data, glob)
    for artifact in artifacts:
        if verbose:
            print_now(artifact.location)
        else:
            print_now(artifact.file_name)


def retrieve_artifact(credentials, url, local_path):
    auth_location = url.replace('http://',
                                'http://{}:{}@'.format(*credentials))
    urllib.urlretrieve(auth_location, local_path)


def get_juju_bin_artifact(package_namer, version, build_data):
    file_name = package_namer.get_release_package(version)
    return get_filename_artifact(file_name, build_data)


def get_filename_artifact(file_name, build_data):
    by_filename = dict((a['fileName'], a) for a in build_data['artifacts'])
    bin_artifact = by_filename[file_name]
    return make_artifact(build_data, bin_artifact)


def retrieve_buildvars(credentials, build_number):
    build_data = get_build_data(JENKINS_URL, credentials, BUILD_REVISION,
                                build_number)
    artifact = get_filename_artifact('buildvars.json', build_data)
    return get_jenkins_json(credentials, artifact.location)


def get_buildvars(credentials, build_number, summary=False, env='unknown',
                  revision_build=False, version=False, branch=False,
                  short_branch=False, revision=False, short_revision=False):
    """Return requested information as text.

    The 'summary' kwarg returns intelligible information about the build.
    'env' is included in the summary text. Otherwise, a space-separated
    string composed of the each True kwarg is returned in this order:
        revision_build version short_branch short_revision branch revision
    Note that revision_build is always the same as build number;
    build_number can be an alias like 'lastBuild' or 'lastSuccessfulBuild'.
    """
    buildvars = retrieve_buildvars(credentials, build_number)
    buildvars['short_revision_id'] = buildvars['revision_id'][0:7]
    buildvars['short_branch'] = buildvars['branch'].split(':')[1]
    buildvars['env'] = env
    template = (
        'Testing {branch} {short_revision_id} on {env} for {revision_build}')
    if summary:
        text = template.format(**buildvars)
    else:
        data = []
        if revision_build:
            data.append(buildvars['revision_build'])
        if version:
            data.append(buildvars['version'])
        if short_branch:
            data.append(buildvars['short_branch'])
        if short_revision:
            data.append(buildvars['short_revision_id'])
        if branch:
            data.append(buildvars['branch'])
        if revision:
            data.append(buildvars['revision_id'])
        text = ' '.join(data)
    return text


def get_release_package_filename(credentials, build_data):
    revision_build = get_revision_build(build_data)
    version = retrieve_buildvars(credentials, revision_build)['version']
    return PackageNamer.factory().get_release_package(version)


def get_juju_bin(credentials, workspace):
    binary_job = JobNamer.factory().get_build_binary_job()
    build_data = get_build_data(JENKINS_URL, credentials, binary_job,
                                'lastBuild')
    file_name = get_release_package_filename(credentials, build_data)
    return get_juju_binary(credentials, file_name, build_data, workspace)


def get_certification_bin(credentials, version, workspace):
    build_data = get_build_data(JENKINS_URL, credentials,
                                CERTIFY_UBUNTU_PACKAGES, 'lastBuild')
    file_name = PackageNamer.factory().get_certification_package(version)
    return get_juju_binary(credentials, file_name, build_data, workspace)


def get_juju_binary(credentials, file_name, build_data, workspace):
    artifact = get_filename_artifact(file_name, build_data)
    target_path = os.path.join(workspace, artifact.file_name)
    retrieve_artifact(credentials, artifact.location, target_path)
    return acquire_binary(target_path, workspace)


def acquire_binary(package_path, workspace):
    bin_dir = os.path.join(workspace, 'extracted-bin')
    extract_deb(package_path, bin_dir)
    for root, dirs, files in os.walk(bin_dir):
        if 'juju' in files:
            return os.path.join(root, 'juju')


def get_artifacts(credentials, job_name, build, glob, path,
                  archive=False, dry_run=False, verbose=False):
    full_path = os.path.expanduser(path)
    if archive:
        if verbose:
            print_now('Cleaning %s' % full_path)
        if not os.path.isdir(full_path):
            raise ValueError('%s does not exist' % full_path)
        shutil.rmtree(full_path)
        os.makedirs(full_path)
    build_data = get_build_data(JENKINS_URL, credentials, job_name, build)
    artifacts = find_artifacts(build_data, glob)
    for artifact in artifacts:
        local_path = os.path.abspath(
            os.path.join(full_path, artifact.file_name))
        if verbose:
            print_now('Retrieving %s => %s' % (artifact.location, local_path))
        else:
            print_now(artifact.file_name)
        if not dry_run:
            retrieve_artifact(credentials, artifact.location, local_path)
    return artifacts


def clean_environment(env_name, verbose=False):
    try:
        client = client_from_config(env_name, None)
    except NoSuchEnvironment as e:
        # Nothing to do.
        if verbose:
            print_now(str(e))
        return False
    if verbose:
        print_now("Destroying %s" % env_name)
    destroy_environment(client, env_name)
    return True


def setup_workspace(workspace_dir, env=None, dry_run=False, verbose=False):
    """Clean the workspace directory and create an artifacts sub directory."""
    for root, dirs, files in os.walk(workspace_dir):
        for name in files:
            print_now('Removing %s' % name)
            if not dry_run:
                os.remove(os.path.join(root, name))
        for name in dirs:
            print_now('Removing %s' % name)
            if not dry_run:
                shutil.rmtree(os.path.join(root, name))
    artifacts_path = os.path.join(workspace_dir, 'artifacts')
    print_now('Creating artifacts dir.')
    if not dry_run:
        os.mkdir(artifacts_path)
    # "touch empty" to convince jenkins there is an archive.
    empty_path = os.path.join(artifacts_path, 'empty')
    if not dry_run:
        with open(empty_path, 'w'):
            pass
    if env is not None and not dry_run:
        clean_environment(env, verbose=verbose)


def add_artifacts(workspace_dir, globs, dry_run=False, verbose=False):
    """Find files beneath the workspace_dir and move them to the artifacts.

    The list of globs can match the full file name, part of a name, or
    a sub directory: eg: buildvars.json, *.deb, tmp/*.deb.
    """
    workspace_dir = os.path.realpath(workspace_dir)
    artifacts_dir = os.path.join(workspace_dir, 'artifacts')
    for root, dirs, files in os.walk(workspace_dir):
        # create a pseudo-relative path to make glob matches easy.
        relative = os.path.relpath(root, workspace_dir)
        if relative == '.':
            relative = ''
        if 'artifacts' in dirs:
            dirs.remove('artifacts')
        for file_name in files:
            file_path = os.path.join(root, file_name)
            file_relative_path = os.path.join(relative, file_name)
            for glob in globs:
                if fnmatch.fnmatch(file_relative_path, glob):
                    if verbose:
                        print_now("Adding artifact %s" % file_relative_path)
                    if not dry_run:
                        shutil.move(file_path, artifacts_dir)
                    break


def add_build_job_glob(parser):
    """Added the --build, job, and glob arguments to the parser."""
    parser.add_argument(
        '-b', '--build', default='lastSuccessfulBuild',
        help="The specific build to examine (default: lastSuccessfulBuild).")
    parser.add_argument(
        'job', help="The job that collected the artifacts.")
    parser.add_argument(
        'glob', nargs='?', default='*',
        help="The glob pattern to match artifact file names.")


def add_credential_args(parser):
    parser.add_argument(
        '--user', default=os.environ.get('JENKINS_USER'))
    parser.add_argument(
        '--password', default=os.environ.get('JENKINS_PASSWORD'))


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("List and get artifacts from Juju CI.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    parser_list = subparsers.add_parser(
        'list', help='list artifacts for a job build')
    add_build_job_glob(parser_list)
    parser_get = subparsers.add_parser(
        'get', help='get artifacts for a job build')
    add_build_job_glob(parser_get)
    parser_get.add_argument(
        '-a', '--archive', action='store_true', default=False,
        help='Ensure the download path exists and remove older files.')
    parser_get.add_argument(
        'path', nargs='?', default='.',
        help="The path to download the files to.")
    add_credential_args(parser_list)
    add_credential_args(parser_get)
    parser_workspace = subparsers.add_parser(
        'setup-workspace', help='Setup and clean a workspace for building.')
    parser_workspace.add_argument(
        '-e', '--clean-env', dest='clean_env', default=None,
        help='Ensure the env resources are freed or deleted.')
    parser_workspace.add_argument(
        'path', help="The path to the existing workspace directory.")
    parser_get_juju_bin = subparsers.add_parser(
        'get-juju-bin', help='Retrieve and extract juju binaries.')
    parser_get_juju_bin.add_argument('workspace', nargs='?', default='.',
                                     help='The place to store binaries.')
    add_credential_args(parser_get_juju_bin)
    parser_get_certification_bin = subparsers.add_parser(
        'get-certification-bin',
        help='Retrieve and extract juju binaries for certification.')
    parser_get_certification_bin.add_argument(
        'version', help='The version to get certification for.')
    parser_get_certification_bin.add_argument(
        'workspace', nargs='?', default='.',
        help='The place to store binaries.')
    add_credential_args(parser_get_certification_bin)
    parser_get_buildvars = subparsers.add_parser(
        'get-build-vars',
        help='Retrieve the build-vars for a build-revision.')
    parser_get_buildvars.add_argument(
        '--env', default='Unknown',
        help='The env name to include in the summary')
    parser_get_buildvars.add_argument(
        '--summary', action='store_true', default=False,
        help='Summarise the build var test data')
    parser_get_buildvars.add_argument(
        '--revision-build', action='store_true', default=False,
        help='Print the test revision build number')
    parser_get_buildvars.add_argument(
        '--version', action='store_true', default=False,
        help='Print the test juju version')
    parser_get_buildvars.add_argument(
        '--short-branch', action='store_true', default=False,
        help='Print the short name of the branch')
    parser_get_buildvars.add_argument(
        '--short-revision', action='store_true', default=False,
        help='Print the short revision of the branch')
    parser_get_buildvars.add_argument(
        '--branch', action='store_true', default=False,
        help='Print the test branch')
    parser_get_buildvars.add_argument(
        '--revision', action='store_true', default=False,
        help='Print the test revision')
    parser_get_buildvars.add_argument(
        'build', help='The build-revision build number')
    add_credential_args(parser_get_buildvars)
    parser_get_package_name = subparsers.add_parser(
        'get-package-name',
        help='Determine the package name for the current machine.')
    parser_get_package_name.add_argument('version', help='The version to use.')
    parsed_args = parser.parse_args(args)
    if parsed_args.command == 'get-build-vars' and True not in (
            parsed_args.summary, parsed_args.revision_build,
            parsed_args.version, parsed_args.revision,
            parsed_args.short_branch, parsed_args.short_revision,
            parsed_args.branch, parsed_args.revision):
        parser_get_buildvars.error(
            'Expected --summary or one or more of: --revision-build, '
            '--version, --revision, --short-branch, --short-revision, '
            '--branch, --revision')
    credentials = get_credentials(parsed_args)
    return parsed_args, credentials


def get_credentials(args):
    if 'user' not in args:
        return None
    if None in (args.user, args.password):
        raise CredentialsMissing(
            'Jenkins username and/or password not supplied.')
        return None
    return Credentials(args.user, args.password)


class Namer:
    """A base class that has distro and arch info used to name things."""

    @classmethod
    def factory(cls):
        dist_info = get_distro_information()
        return cls(get_deb_arch(), dist_info['RELEASE'], dist_info['CODENAME'])

    def __init__(self, arch, distro_release, distro_series):
        self.arch = arch
        self.distro_release = distro_release
        self.distro_series = distro_series


class PackageNamer(Namer):
    """A class knows the names of packages."""

    def get_release_package_suffix(self):
        return '-0ubuntu1~{distro_release}.1~juju1_{arch}.deb'.format(
            distro_release=self.distro_release, arch=self.arch)

    def get_release_package(self, version):
        return (
            'juju-core_{version}{suffix}'
            ).format(version=version, suffix=self.get_release_package_suffix())

    def get_certification_package(self, version):
        return (
            'juju-core_{version}~{distro_release}.1_{arch}.deb'
            ).format(version=version, distro_release=self.distro_release,
                     arch=self.arch)


class JobNamer(Namer):
    """A class knows the names of jobs."""

    def get_build_binary_job(self):
        return 'build-binary-{distro_series}-{arch}'.format(
            distro_series=self.distro_series, arch=self.arch)


def main(argv):
    """Manage list and get files from Juju CI builds."""
    try:
        args, credentials = parse_args(argv)
    except CredentialsMissing as e:
        print_now(e)
        sys.exit(2)
    try:
        if args.command == 'list':
            list_artifacts(
                credentials, args.job, args.build, args.glob,
                verbose=args.verbose)
        elif args.command == 'get':
            get_artifacts(
                credentials, args.job, args.build, args.glob, args.path,
                archive=args.archive, dry_run=args.dry_run,
                verbose=args.verbose)
        elif args.command == 'setup-workspace':
            setup_workspace(
                args.path, env=args.clean_env,
                dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'get-juju-bin':
            print_now(get_juju_bin(credentials, args.workspace))
        elif args.command == 'get-certification-bin':
            path = get_certification_bin(credentials, args.version,
                                         args.workspace)
            print_now(path)
        elif args.command == 'get-build-vars':
            text = get_buildvars(
                credentials, args.build, env=args.env,
                summary=args.summary, revision_build=args.revision_build,
                version=args.version, short_branch=args.short_branch,
                short_revision=args.short_revision,
                branch=args.branch, revision=args.revision)
            print_now(text)
        elif args.command == 'get-package-name':
            print_now(PackageNamer.factory().get_release_package(args.version))

    except Exception as e:
        print_now(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print_now("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
