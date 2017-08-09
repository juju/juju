// Pipeline configuration for a Juju CI Run
// Intended to be run when a feature branch is merged.
// Builds binaries and runs functional (and unit) tests.

node('juju-core-slave') {
    // setup working directory and script dir paths (for after scm checkout.)
    work_dir = "${pwd(tmp: true)}/${env.BUILD_NUMBER}"
    // juju_build_dir = "${work_dir}/build/"
    // juju_checkout_dir = "$build_base_dir/src/github.com/juju/juju"

    stage('Build') {
        def arch = "amd64"  // as we'll parallise this stuff.
        parallel(
            'amd64': {
                node('build-slave') {
                    sh("""
                    sudo snap install go --classic \
                    && sudo apt-get install snapcraft -y
                    """)

                    dir(${pwd(tmp: true)}) {
                        checkout scm
                        withEnv(['PATH+SNAP=/snap/bin']) {
                            sh "go version"
                            sh "snapcraft build && snapcraft prime"
                            // Also, artifact the snap itself
                            stash includes: "./prime/bin/juju*", name: "${arch}-binaries"
                        }
                    }
                }
            },
        )
    }

    stage('Test') {
        unstash 'amd64-binaries'
        print(sh(script: "ls -larth", returnStdout: true))
    }
}