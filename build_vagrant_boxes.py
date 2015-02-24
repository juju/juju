#!/usr/bin/env python
from argparse import ArgumentParser
import logging
import os
import shutil
import subprocess
import sys

from jujuci import (
    add_credential_args,
    get_artifacts,
    get_credentials,
    PUBLISH_REVISION
)

"""Build Juju-Vagrant boxes for Juju packages build by publish-revision.

Required environment variables:

    WORKSPACE - path to Jenkins' workspace directory
    JENKINS_KVM - path to the local copy of
        lp:~ubuntu-on-ec2/vmbuilder/jenkins_kvm (main build scripts)
"""

SERIES_TO_NUMBERS = {
    'trusty': '14.04',
    'precise': '12.04',
}
JENKINS_KVM = 'JENKINS_KVM'
WORKSPACE = 'WORKSPACE'


def get_package_globs(series, arch):
    series_number = SERIES_TO_NUMBERS[series]
    return {
        'core': 'juju-core_*%s*_%s.deb' % (series_number, arch),
        'local': 'juju-local_*%s*_all.deb' % series_number,
    }


def get_debian_packages(credentials, workspace, series, arch, revision_build):
    result = {}
    package_globs = get_package_globs(series, arch)
    try:
        for package, glob in package_globs.items():
            artifacts = get_artifacts(
                credentials, PUBLISH_REVISION, revision_build, glob,
                workspace, dry_run=False, verbose=False)
            file_path = os.path.join(workspace, artifacts[0].file_name)
            result[package] = file_path
    except Exception:
        for file_path in result.values():
            if os.path.exists(file_path):
                os.unlink(file_path)
        raise
    return result


def remove_leftover_virtualbox(series, arch):
    """The build script sometimes does not remove a VirtualBox instance.

    If such an instance is not deleted, the next build for the same
    series/architecture will fail.
    """
    left_over_name = 'ubuntu-cloudimg-%s-juju-vagrant-%s' % (series, arch)
    instances = subprocess.check_output(['vboxmanage', 'list', 'vms'])
    for line in instances.split('\n'):
        if line != '':
            name, uid = line.split(' ')
            name = name.strip('"')
            if name == left_over_name:
                subprocess.check_call([
                    'vboxmanage', 'unregistervm', name, '--delete'])
                break


def build_vagrant_box(series, arch, jenkins_kvm, workspace, package_info=None):
    env = os.environ.copy()
    env['BUILD_FOR'] = '%s:%s' % (series, arch)
    if package_info is not None:
        env['JUJU_CORE_PKG'] = package_info['core']
        env['JUJU_LOCAL_PKG'] = package_info['local']
    builder_path = os.path.join(jenkins_kvm, 'build-juju-local.sh')
    build_dir = '%s-%s' % (series, arch)
    logging.info('Building Vagrant box for %s %s' % (series, arch))
    try:
        subprocess.check_call(builder_path, env=env)
        boxname = '%s-server-cloudimg-%s-juju-vagrant-disk1.box' % (
            series, arch)
        os.rename(
            os.path.join(jenkins_kvm, build_dir, boxname),
            os.path.join(workspace, boxname))
    finally:
        remove_leftover_virtualbox(series, arch)
        full_build_dir_path = os.path.join(jenkins_kvm, build_dir)
        if os.path.exists(full_build_dir_path):
            shutil.rmtree(full_build_dir_path)
        builder_image = os.path.join(
            jenkins_kvm, '%s-builder-%s.img' % (series, arch))
        if os.path.exists(builder_image):
            os.unlink(builder_image)


def clean_workspace(workspace):
    """Remove any files and directories found in the workspace."""
    for item in os.listdir(workspace):
        path = os.path.join(workspace, item)
        if os.path.isdir(path):
            shutil.rmtree(path)
        else:
            os.unlink(path)


def main():
    logging.basicConfig(
        level=logging.INFO, format='%(asctime)s %(levelname)s %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S')
    parser = ArgumentParser('Build Juju-Vagrant boxes')
    parser.add_argument(
        '--jenkins-kvm', help='Directory with the main build script',
        required=True)
    parser.add_argument(
        '--workspace', help='Workspace directory', required=True)
    parser.add_argument(
        '--series', help='Build the image for this series', required=True)
    parser.add_argument(
        '--arch', help='Build the image for this architecture', required=True)
    parser.add_argument(
        '--use-ci-juju-packages',
        help=(
            'Build the image with juju packages built by CI. The value of '
            'this option is a revision_build number. The debian packages '
            'are retrieved from a run of the publish-revision for this '
            'revision_build.'))
    add_credential_args(parser)
    args = parser.parse_args()
    clean_workspace(args.workspace)
    if args.use_ci_juju_packages is not None:
        credentials = get_credentials(args)
        package_info = get_debian_packages(
            credentials, args.workspace, args.series, args.arch,
            args.use_ci_juju_packages)
        if 'core' not in package_info:
            logging.error('Could not find juju-core package')
            sys.exit(1)
        if 'local' not in package_info:
            logging.error('Could not find juju-local package')
            sys.exit(1)
    else:
        package_info = None
    try:
        build_vagrant_box(
            args.series, args.arch, args.jenkins_kvm, args.workspace,
            package_info)
    finally:
        if package_info is not None:
            for filename in package_info.values():
                os.unlink(filename)


if __name__ == '__main__':
    main()
