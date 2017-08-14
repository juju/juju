// Pipeline configuration for a Juju CI Run
// Intended to be run when a feature branch is merged.
// Builds binaries and runs functional (and unit) tests.

node('juju-core-slave') {
    // setup working directory and script dir paths (for after scm checkout.)
    work_dir = "${pwd(tmp: true)}/${env.BUILD_NUMBER}"
    amd64_juju_data = "${pwd(tmp: true)}/amd64-juju"

    stage('Build') {
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

                parallel(
                    'log-rotation': {
                        node('feature-slave-a') {
                            working_path = "${pwd()}/amd64"
                            bin_path = "${working_path}/bin"
                            logs_path = "${working_path}/logs/log-rotation"
                            juju_data_base = "${working_path}/juju_data"
                            scripts_path = "${working_path}/acceptancetests"
                            cloud_city = "${working_path}/cloud-city"
                            env='parallel-lxd'
                            job_name='log-rotation-ci-run'

                            dir(working_path) {
                                unstash 'acceptancetests'
                                // safe create some directories
                                dir(bin_path) {}
                                dir(logs_path) {
                                    // This is because workspaces are re-used?
                                    // Do they survive a log archiving?
                                    sh "rm -fr ./*"
                                }

                                get_cloud_city()
                            }

                            dir(bin_path) {
                                unstash 'amd64-binaries'
                            }

                            juju_data = "${juju_data_base}/log-rotation"
                            dir(juju_data) {
                                withEnv(['JUJU_DATA=${juju_data}']) {
                                    // scripts/blah logs/ ...
                                    withEnv(["JUJU_HOME=${cloud_city}", "JUJU_REPOSITORY=${scripts_path}/repository"]) {
                                        ret_code = sh(
                                            script: "${scripts_path}/assess_log_rotation.py ${env} ${bin_path}/juju ${logs_path} ${job_name} machine --series xenial",
                                            returnStatus: true)

                                    }

                                    if(ret_code != 0) {
                                        error "log-rotation test failed."
                                    }
                                }
                            }
                            archiveArtifacts(
                                artifacts: "amd64/logs/log-rotation/",
                                fingerprint: true)

                    },
                    // 'model-migration': {
                    //         node('feature-slave-b') {
                    //             working_path = "${pwd()}/amd64"
                    //             bin_path = "${working_path}/bin"
                    //             logs_path = "${working_path}/logs"
                    //             juju_data_base = "${working_path}/juju_data"
                    //             scripts_path = "${working_path}/acceptancetests"

                    //             dir(working_path) {
                    //                 unstash 'acceptancetests'
                    //                 // safe create some directories
                    //                 dir(bin_path) {}
                    //                 dir(logs_path) {
                    //                     sh "rm -fr ./*"
                    //                 }

                    //                 get_cloud_city()
                    //             }

                    //             dir(bin_path) {
                    //                 unstash 'amd64-binaries'
                    //             }

                    //             juju_data = "${juju_data_base}/model-migration"
                    //             dir(juju_data) {
                    //                 withEnv(['JUJU_DATA=${juju_data}']) {

                    //                     sh "${bin_path}/juju version"
                    //                     // scripts/blah logs/ ...
                    //                     // artifact logs/ ...
                    //                 }
                    //             }
                    //     }
                    // }
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
