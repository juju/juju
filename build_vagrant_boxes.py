#!/usr/bin/env python
from argparse import ArgumentParser
from jenkins import Jenkins
import logging
import os
import re
import shutil
import subprocess
import sys


"""Build Juju-Vagrant boxes for Juju packages build by publish-revision.

Required environment variables:

    WORKSPACE - path to Jenkins' workspace directory
    JENKINS_KVM - path to the local copy of
        lp:~ubuntu-on-ec2/vmbuilder/jenkins_kvm (main build scripts)
"""

JENKINS_URL = 'http://juju-ci.vapour.ws:8080'
PUBLISH_REVISION_JOB = 'publish-revision'
SERIES_TO_NUMBERS = {
    'trusty': '14.04',
    'precise': '12.04',
}
JENKINS_KVM = 'JENKINS_KVM'
WORKSPACE = 'WORKSPACE'

def package_regexes(series, arch):
    series_number = SERIES_TO_NUMBERS[series].replace('.', r'\.')
    regex_core = re.compile(
        r'^juju-core_.*%s.*%s\.deb$' % (series_number, arch))
    regex_local = re.compile(
        r'^juju-local_.*%s.*all\.deb$' % series_number)
    return {
        'core': regex_core,
        'local': regex_local,
    }


def get_debian_packages(jenkins, workspace, series, arch):
    job_info = jenkins.get_job_info(PUBLISH_REVISION_JOB)
    build_number = job_info['lastSuccessfulBuild']['number']
    build_info = jenkins.get_build_info(PUBLISH_REVISION_JOB, build_number)

    result = {}
    regexes = package_regexes(series, arch)
    try:
        for artifact in build_info['artifacts']:
            filename = artifact['fileName']
            for package, matcher in regexes.items():
                if matcher.search(filename) is not None:
                    package_url = '%s/artifact/%s' % (
                        build_info['url'], filename)
                    local_path = os.path.join(workspace, filename)
                    logging.info(
                        'copying %s from build %s' % (filename, build_number))
                    result[package] = local_path
                    command = 'wget -q -O %s %s' % (
                        local_path, package_url)
                    subprocess.check_call(command.split(' '))
                    break
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
        name, uid = line.split(' ')
        name = name.strip('"')
        if name == left_over_name:
            subprocess.check_call([
                'vboxmanage', 'unregistervm', name, '--delete'])
            break


def build_vagrant_box(series, arch, juju_core_package, juju_local_package,
                      jenkins_kvm, workspace):
    env = {
        'JUJU_CORE_PKG': juju_core_package,
        'JUJU_LOCAL_PKG': juju_local_package,
        'BUILD_FOR': '%s:%s' % (series, arch),
    }
    builder_path = os.path.join(jenkins_kvm, 'build-juju-local.sh')
    logging.info('Building Vagrant box for %s %s' % (series, arch))
    try:
        subprocess.check_call(builder_path, env=env)
        boxname = '%s-server-cloudimg-%s-juju-vagrant-disk1.box' % (
            series, arch)
        build_dir = '%s-%s' % (series, arch)
        os.rename(
            os.path.join(jenkins_kvm, build_dir, boxname),
            os.path.join(workspace, boxname))
    finally:
        remove_leftover_virtualbox(series, arch)
        shutil.rmtree(os.path.join(jenkins_kvm, build_dir))
        os.unlink(os.path.join(
            jenkins_kvm, '%s-builder-%s.img' % (series, arch)))


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
    args = parser.parse_args()
    clean_workspace(args.workspace)
    jenkins = Jenkins(JENKINS_URL)
    package_info = get_debian_packages(
        jenkins, args.workspace, args.series, args.arch)
    if 'core' not in package_info:
        logging.error('Could not find juju-core package')
        sys.exit(1)
    if 'local' not in package_info:
        logging.error('Could not find juju-local package')
        sys.exit(1)
    try:
        build_vagrant_box(
            args.series, args.arch, package_info['core'], package_info['local'],
            args.jenkins_kvm, args.workspace)
    finally:
        for filename in package_info.values():
            os.unlink(filename)


if __name__ == '__main__':
    main()
