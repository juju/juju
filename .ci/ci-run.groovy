// Pipeline configuration for a Juju CI Run
// Intended to be run when a feature branch is merged.
// Builds binaries and runs functional (and unit) tests.

node('juju-core-slave') {
    // setup working directory and script dir paths (for after scm checkout.)
    work_dir = "${pwd(tmp: true)}/${env.BUILD_NUMBER}"
    amd64_juju_data = "${pwd(tmp: true)}/amd64-juju"

    stage('Build') {
        // Grab source and save test scripts for later.
        checkout scm
        stash includes: "acceptancetests/**/*", name: "acceptancetests"
        // A re-run of the binary build should be fine as the stuff won't have
        // been cleaned up? Or should we clean up properly etc.
        parallel(
            'amd64': {
                node('build-slave') {
                    binary_stash_name = build_binaries('amd64')
                }
            },
            // 's390x': {
            //     node('s390x-slave') {
            //         binary_stash_name = build_binaries('s390x')
            //     }
            // }
        )
    }

    stage('Test') {
        parallel(
            'amd64': {
                def working_path = "${pwd()}"
                def arch = 'amd64'

                parallel(
                    'log-rotation-machine': {
                        node('feature-slave-a') {
                            timeout(time: 25, unit: 'MINUTES') {
                                run_simple_test(
                                    'log-rotation',
                                    'parallel-lxd',
                                    working_path,
                                    arch,
                                    'assess_log_rotation.py',
                                    'machine --series xenial',
                                )
                            }
                        }

                    },
                    'model-migration': {
                        node('feature-slave-b') {
                            timeout(time: 50, unit: 'MINUTES') {
                                run_simple_test(
                                    'model-migration',
                                    'parallel-abc',
                                    working_path,
                                    arch,
                                    'assess_model_migration.py',
                                    '--timeout 2700',
                                )
                            }
                        }
                    }
                )
            },// amd64
            // 's390x': {
            //     node('s390x-slave') {
            //         binary_parh = "${pwd()}/juju_binary"
            //         dir(binary_path) {
            //             unstash 's390x-binaries'
            //             sh "${binary_path}/juju version"
            //         }
            //     }

            // }
        )
    }
}


def build_binaries(arch) {
    // Note: this will have issues if multiple runs attempt to run this step
    // on the same machine.
    def binary_stash_name = "${arch}-binaries"
    sh("""
    sudo snap install go --classic \
    && sudo apt-get install snapcraft -y
    """)

    //dir("${pwd(tmp: true)}") {
    dir("${pwd()}/_build") {
        checkout scm
        withEnv(['PATH+SNAP=/snap/bin']) {
            sh "go version"
            sh "snapcraft build && snapcraft prime"
            // Also, artifact the snap itself
            dir('./prime/bin/') {
                stash includes: "juju*", name: "${binary_stash_name}"
            }
        }
    }

    return binary_stash_name
}


def get_cloud_city() {
    // Checkout cloud city into the current directory
    checkout(
        changelog: false,
        poll: false,
        scm: [
            $class: 'GitSCM',
            branches: [[name: '*/master']],
            doGenerateSubmoduleConfigurations: false,
            extensions: [[$class: 'RelativeTargetDirectory', relativeTargetDir: 'cloud-city']],
            submoduleCfg: [],
            userRemoteConfigs: [[url: 'git+ssh://juju-qa-bot@git.launchpad.net/~juju-qa/+git/cloud-city']]]
    )
}


def prepare_simple_test(working_path, test_name, arch) {
    // This is a little gross, we can make it better I'm sure.
    def bin_path = "${working_path}/${arch}/bin"
    def juju_data_base = "${working_path}/${arch}/juju_data"
    def juju_data = "${juju_data_base}/${arch}/${test_name}"
    def scripts_path = "${working_path}/${arch}/acceptancetests"
    def cloud_city = "${working_path}/${arch}/cloud-city"
    def base_logs_path = "${working_path}/${arch}/logs"
    def logs_collection = "${arch}/logs/${test_name}"
    def logs_path = "${working_path}/${logs_collection}"

    dir(bin_path) {
        if(! fileExists('.binary-prepared')) {
            unstash "${arch}-binaries"
            sh 'touch .binary-prepared'
        }
    }

    dir(juju_data_base) {}
    dir(working_path) {
        get_cloud_city()

        if(! fileExists('.acceptancetests-prepared')) {
            unstash 'acceptancetests'
            sh 'touch .acceptancetests-prepared'
        }
    }
    dir(logs_path) {
        // This is unique to the named test (not shared on a node.)
        sh "rm -fr ./* || true"
        sh "touch .log-${test_name}"
    }

    return [
        bin_path: bin_path,
        juju_data_base: juju_data_base,
        juju_data: juju_data,
        scripts_path: scripts_path,
        cloud_city: cloud_city,
        base_logs_path: base_logs_path,
        logs_path: logs_path,
        logs_collection: logs_collection,
    ]
}


def run_simple_test(test_name, env, working_path, arch, test_script, extra_args) {
    def run_paths = prepare_simple_test(
        working_path, test_name, arch)
    def job_name = "${test_name}-ci-run"

    try {
        dir(run_paths['juju_data']) {
            withEnv(["JUJU_DATA=${run_paths['juju_data']}"]) {
                withEnv([
                    "JUJU_HOME=${run_paths['cloud_city']}",
                    "JUJU_REPOSITORY=${run_paths['scripts_path']}/repository"
                ]) {
                    def ret_code = sh(
                        script: "${run_paths['scripts_path']}/${test_script} ${env} ${run_paths['bin_path']}/juju ${run_paths['logs_path']} ${job_name} ${extra_args}",
                        returnStatus: true)

                    if(ret_code != 0) {
                        error "${test_name} test failed."
                    }

                }
            }
        }
    } finally {
        // when this fails it sets currentBuild.result to failed.
        archiveArtifacts(
            artifacts: run_paths['logs_collection'],
            fingerprint: true)
    }
}