#!/usr/bin/python
"""Manage the blessed juju revision testing candiates."""

from __future__ import print_function

from argparse import ArgumentParser
import datetime
import json
import os
import shutil
import subprocess
import sys
import traceback

from jujuci import (
    add_credential_args,
    BUILD_REVISION,
    get_build_data,
    get_artifacts,
    get_credentials,
    JENKINS_URL,
    PUBLISH_REVISION
)
from utility import (
    extract_deb,
    get_deb_arch,
    get_revision_build,
    run_command,
    s3_cmd,
    temp_dir,
)


def find_publish_revision_number(credentials, br_number, limit=20):
    """Return the publish-revsion number paired with build-revision number."""
    found_number = None
    job_number = 'lastSuccessfulBuild'
    for i in range(limit):
        build_data = get_build_data(
            JENKINS_URL, credentials, PUBLISH_REVISION, build=job_number)
        if not build_data:
            return None
        # Ensure we have the real job number (an int), not an alias.
        job_number = build_data['number']
        if get_revision_build(build_data) == str(br_number):
            found_number = job_number
            break
        job_number = job_number - 1
    return found_number


def prepare_dir(dir_path, dry_run=False, verbose=False):
    """Create to clean a directory."""
    if os.path.isdir(dir_path):
        if verbose:
            print('Cleaning %s' % dir_path)
        if not dry_run:
            shutil.rmtree(dir_path)
    else:
        if verbose:
            print('Creating %s' % dir_path)
    if not dry_run:
        os.makedirs(dir_path)


def download_candidate_files(credentials, release_number, path, br_number,
                             pr_number=None, dry_run=False, verbose=False):
    """Download the files from the build-revision and publish-revision jobs.

    The buildvars.json for the specific build-revision number is downloaded.
    All the binary and source packages from the last successful build of
    publish revision are downloaded.
    """
    artifact_dir_name = '%s-artifacts' % release_number
    candidate_dir = os.path.join(path, artifact_dir_name)
    prepare_dir(candidate_dir, dry_run, verbose)
    get_artifacts(
        credentials, BUILD_REVISION, br_number, 'buildvars.json',
        candidate_dir, dry_run=dry_run, verbose=verbose)
    if not pr_number:
        pr_number = find_publish_revision_number(credentials, br_number)
    get_artifacts(
        credentials, PUBLISH_REVISION, pr_number, 'juju-core*', candidate_dir,
        dry_run=dry_run, verbose=verbose)


def get_artifact_dirs(path):
    """List the directories that contain artifacts."""
    dirs = []
    for name in os.listdir(path):
        artifacts_path = os.path.join(path, name)
        if name.endswith('-artifacts') and os.path.isdir(artifacts_path):
            dirs.append(name)
    return dirs


def get_package(artifacts_path, version):
    """Return the path to the expected juju-core package for the localhost."""
    release = subprocess.check_output(['lsb_release', '-sr']).strip()
    arch = get_deb_arch()
    package_name = 'juju-core_{}-0ubuntu1~{}.1~juju1_{}.deb'.format(
        version, release, arch)
    package_path = os.path.join(artifacts_path, package_name)
    return package_path


def extract_candidates(path, dry_run=False, verbose=False):
    """Extract all the candidate juju binaries for the local machine.

    Each candidate will be extracted to a directory named after the version
    the artifacts (packages) were made from. Thus the package that matches
    the localhost's series and architecture in the master-artifacts/ directory
    will be extracted to a sibling directory named "master/" The buildvars.json
    data will be copied to the top of "master" to provide information about
    the origin of the binaries.
    """
    for dir_name in get_artifact_dirs(path):
        artifacts_path = os.path.join(path, dir_name)
        buildvars_path = os.path.join(artifacts_path, 'buildvars.json')
        with open(buildvars_path) as bf:
            buildvars = json.load(bf)
        version = buildvars['version']
        package_path = get_package(artifacts_path, version)
        candidate_path = os.path.join(path, version)
        if verbose:
            print('extracting %s to %s' % (package_path, candidate_path))
        prepare_dir(candidate_path, dry_run, verbose)
        if not dry_run:
            extract_deb(package_path, candidate_path)
        if verbose:
            print('Copying %s to %s' % (buildvars_path, candidate_path))
        if not dry_run:
            new_path = os.path.join(candidate_path, 'buildvars.json')
            shutil.copyfile(buildvars_path, new_path)
            shutil.copystat(buildvars_path, new_path)


def get_scripts(juju_release_tools=None):
    """Return a tuple paths to the assemble_script and publish_scripts."""
    assemble_script = 'assemble-streams.bash'
    publish_script = 'publish-public-tools.bash'
    if juju_release_tools:
        assemble_script = os.path.join(
            juju_release_tools, assemble_script)
        publish_script = os.path.join(
            juju_release_tools, publish_script)
    return assemble_script, publish_script


def publish_candidates(path, streams_path,
                       juju_release_tools=None, dry_run=False, verbose=False):
    """Assemble and publish weekly streams from the candidates."""
    timestamp = datetime.datetime.utcnow().strftime('%Y_%m_%dT%H_%M_%S')
    with temp_dir() as debs_path:
        for dir_name in get_artifact_dirs(path):
            artifacts_path = os.path.join(path, dir_name)
            branch_name = dir_name.split('-')[0]
            for deb_name in os.listdir(artifacts_path):
                deb_path = os.path.join(artifacts_path, deb_name)
                if verbose:
                    print('Copying %s' % deb_path)
                new_path = os.path.join(debs_path, deb_name)
                shutil.copyfile(deb_path, new_path)
                if deb_name == 'buildvars.json':
                    # buildvars.json is also in the artifacts_path; copied by
                    # download_candidate_files(). Set it aside so it can be
                    # sync'd to S3 as a record of what was published.
                    buildvar_dir = '{}/weekly/{}/{}'.format(
                        path, timestamp, branch_name)
                    if not os.path.isdir(buildvar_dir):
                        os.makedirs(buildvar_dir)
                    buildvar_path = '{}/{}'.format(buildvar_dir, deb_name)
                    shutil.copyfile(deb_path, buildvar_path)
        assemble_script, publish_script = get_scripts(juju_release_tools)
        # XXX sinzui 2014-12-01: IGNORE uses the local juju, but when
        # testing juju's that change generate-tools, we may need to use
        # the highest version.
        command = [
            assemble_script, '-t', debs_path, 'weekly', 'IGNORE',
            streams_path]
        run_command(command, dry_run=dry_run, verbose=verbose)
    publish(streams_path, publish_script, dry_run=dry_run, verbose=verbose)
    # Sync buildvars.json files out to s3.
    url = 's3://juju-qa-data/juju-releases/weekly/'
    s3_path = '{}/weekly/{}'.format(path, timestamp)
    if verbose:
        print('Calling s3cmd to sync %s out to %s' % (s3_path, url))
    if not dry_run:
        s3_cmd(['sync', s3_path, url])
    extract_candidates(path, dry_run=dry_run, verbose=verbose)


def publish(streams_path, publish_script, dry_run=False, verbose=False):
    juju_dist_path = os.path.join(streams_path, 'juju-dist')
    command = [publish_script, 'weekly', juju_dist_path, 'cpc']
    for attempt in range(3):
        try:
            run_command(command, dry_run=dry_run, verbose=verbose)
            break
        except subprocess.CalledProcessError:
            # Raise an error when the third attempt fails; the cloud is ill.
            if attempt == 2:
                raise


def parse_args(args=None):
    """Return the argument parser for this program."""
    parser = ArgumentParser("Manage the successful Juju CI candidates.")
    parser.add_argument(
        '-d', '--dry-run', action='store_true', default=False,
        help='Do not make changes.')
    parser.add_argument(
        '-v', '--verbose', action='store_true', default=False,
        help='Increase verbosity.')
    subparsers = parser.add_subparsers(help='sub-command help', dest="command")
    # ./candidate download -b 1234 master ~/candidate
    parser_update = subparsers.add_parser(
        'download', help='download a candidate')
    parser_update.add_argument(
        '-b', '--br-number', default='lastSuccessfulBuild',
        help="The specific build-revision number.")
    parser_update.add_argument(
        '-p', '--pr-number',
        help="The specific publish-revision-revision number.")
    parser_update.add_argument(
        'release_number', help='The successfully test branch release number.')
    parser_update.add_argument(
        'path', help='The path to save the candiate data to.')
    add_credential_args(parser_update)
    # ./candidate extract master ~/candidate
    parser_extract = subparsers.add_parser(
        'extract',
        help='extract candidates that match the local series and arch.')
    parser_extract.add_argument(
        'path', help='The path to the candiate data dir.')
    # ./candidate --juju-release-tools $JUJU_RELEASE_TOOLS \
    #    publish ~/candidate ~/streams
    parser_publish = subparsers.add_parser(
        'publish', help='Publish streams for the candidates')
    parser_publish.add_argument(
        '-t', '--juju-release-tools',
        help='The path to the juju-release-tools dir.')
    parser_publish.add_argument(
        'path', help='The path to the candiate data dir.')
    parser_publish.add_argument(
        'streams_path', help='The path to the streams data dir.')
    parsed_args = parser.parse_args(args)
    return parsed_args, get_credentials(parsed_args)


def main(argv):
    """Manage successful Juju CI candiates."""
    args, credentials = parse_args(argv)
    try:
        if args.command == 'download':
            download_candidate_files(
                credentials, args.release_number, args.path, args.br_number,
                args.pr_number, dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'extract':
            extract_candidates(
                args.path, dry_run=args.dry_run, verbose=args.verbose)
        elif args.command == 'publish':
            publish_candidates(
                args.path, args.streams_path,
                juju_release_tools=args.juju_release_tools,
                dry_run=args.dry_run, verbose=args.verbose)
    except Exception as e:
        print(e)
        if args.verbose:
            traceback.print_tb(sys.exc_info()[2])
        return 2
    if args.verbose:
        print("Done.")
    return 0


if __name__ == '__main__':
    sys.exit(main(sys.argv[1:]))
