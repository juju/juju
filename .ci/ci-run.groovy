// Pipeline configuration for a Juju CI Run
// Intended to be run when a feature branch is merged.
// Builds binaries and runs functional (and unit) tests.

node('juju-core-slave') {
    // setup working directory and script dir paths (for after scm checkout.)
    work_dir = "${pwd(tmp: true)}/${env.BUILD_NUMBER}"
    // juju_build_dir = "${work_dir}/build/"
    // juju_checkout_dir = "$build_base_dir/src/github.com/juju/juju"
    amd64_juju_data = "${pwd(tmp: true)}/amd64-juju"

    stage('Build') {
        parallel(
            'amd64': {
                node('build-slave') {
                    // sh("""
                    // sudo snap install go --classic \
                    // && sudo apt-get install snapcraft -y
                    // """)

                    // dir("${pwd(tmp: true)}") {
                    //     checkout scm
                    //     withEnv(['PATH+SNAP=/snap/bin']) {
                    //         sh "go version"
                    //         sh "snapcraft build && snapcraft prime"
                    //         // Also, artifact the snap itself
                    //         dir('./prime/bin/') {
                    //             stash includes: "**/prime/bin/juju*", name: "${arch}-binaries"
                    //         }
                    //     }
                    // }
                    binary_stash_name = build_binaries('amd64')
                }
            },
        )
    }

    stage('Test') {
        parallel(
            'amd64': {
                binary_path = pwd(tmp: true)
                dir(binary_path) {
                    unstash 'amd64-binaries'
                    parallel(
                        'log-rotation': {
                            juju_data = pwd(tmp:true)
                            dir(juju_data) {
                                sh "mkdir _logs"
                                withEnv(['JUJU_DATA=${juju_data}']) {
                                    sh "${binary_path}/juju version"
                                    // scripts/blah logs/ ...
                                    // artifact logs/ ...
                                }
                            }
                        },
                        'model-migration': {
                            juju_data = pwd(tmp:true)
                            dir(juju_data) {
                                sh "mkdir _logs"
                                withEnv(['JUJU_DATA=${juju_data}']) {
                                    sh "${binary_path}/juju version"
                                    // scripts/blah logs/ ...
                                    // artifact logs/ ...
                                }
                            }
                        }
                    )
                } // dir binary path.
            }
        )
    }
}

def build_binaries(arch) {
    def binary_stash_name = "${arch}-binaries"
    sh("""
    sudo snap install go --classic \
    && sudo apt-get install snapcraft -y
    """)

    dir("${pwd(tmp: true)}") {
        checkout scm
        withEnv(['PATH+SNAP=/snap/bin']) {
            sh "go version"
            sh "snapcraft build && snapcraft prime"
            // Also, artifact the snap itself
            dir('./prime/bin/') {
                stash includes: "**/prime/bin/juju*", name: "${binary_stash_name}"
            }
        }
    }

    return binary_stash_name
}

// // Get binaries
// // Bootstrap a controller with a known name for many clouds
// //   using --existing.
// // Can you parallel within a parallel?
// cwd = pwd(tmp: true)
// dir(cwd) {
//     unstash 'amd64-binaries'
//     dir(amd64_juju_data) {
//         parallel(
//             'lxd': {
//                 withEnv(['JUJU_DATA=${amd64_juju_data']) {
//                     // This would be a bootstrap . . .
//                     sh(script: "${cwd}/juju version")
//                 }
//             },
//             'aws': {
//                 withEnv(['JUJU_DATA=${amd64_juju_data']) {
//                     // This would be a bootstrap . . .
//                     sh(script: "${cwd}/juju version")
//                 }
//             }
//         )
//     }
// }