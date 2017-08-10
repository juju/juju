import groovy.json.JsonSlurperClassic

node('juju-core-slave-b') {
    // setup working directory and script dir paths (for after scm checkout.)
    work_dir = "${pwd(tmp: true)}/${env.BUILD_NUMBER}"
    scripts_dir = "${work_dir}/acceptancetests"
    release_scripts = "${work_dir}/releasetests"
    // This is where we will check cloud-city out into.
    cloud_city = "${work_dir}/cloud-city"
    print("Using workdir: ${work_dir}")

    // Don't build any tests for PRs that have the label 'no-test-run'
    skip_building = false

    try {
        stage('Precheck') {
            if(env.CHANGE_TARGET == 'master' || env.CHANGE_TARGET == 'staging') {
                error('No PRs or merges are accepted against this branch. Please submit your PR against either develop or a feature or release branch.')
            }

            def pr_issue_api = "https://api.github.com/repos/juju/juju/issues/${env.CHANGE_ID}".toURL()
            def pr_issue = new JsonSlurperClassic().parseText(pr_issue_api.getText())
            def labels = pr_issue['labels']
            labels.each {
                if(it['name'] == 'no-test-run') {
                    print("Ignoring this build due to label 'no-test-run'")
                    skip_building = true
                }
            }
            if (skip_building) {
                githubNotify(
                    context: 'continuous-integration/jenkins/pr-merge',
                    description: 'Not running CI: Tagged no-test-run',
                    status: 'SUCCESS')
            } else {
                githubNotify(
                    context: 'continuous-integration/jenkins/pr-merge',
                    description: 'CI Run started.',
                    status: 'PENDING')
            }
        }

        stage('Build') {
            if(skip_building) {
                print('Skipping due to no-test-run tag.')
                return
            }

            dir(work_dir) {
                checkout scm

                // Checkout cloud city
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

                sh(script: "${scripts_dir}/clean_lxd.py")

                withEnv(["PATH+GO=/usr/lib/go-1.8/bin/"]) {
                    retcode = sh(
                        script: "${release_scripts}/make-pr-tarball.bash ${env.CHANGE_ID}",
                        returnStatus: true)
                    if(retcode != 0) {
                        error("Failed to build.")
                    }

                    go_src_path = sh(
                        script: "find \"$work_dir/\" -type d -name src -regex '.*juju-core[^/]*/src'",
                        returnStdout: true).trim()
                    go_dir = sh(script: "dirname $go_src_path", returnStdout: true).trim()

                    // env.GOPATH = go_dir
                    try {
                        withEnv(["GOPATH=${go_dir}"]) {
                            sh 'echo Using $GOPATH'
                            sh "go install github.com/juju/juju/..."
                        }
                    } catch(e) {
                        error "Failed to build: go install failed."
                    }
                }
            }
        }

        stage('Testing') {
            if(skip_building) {
                print('Skipping due to no-test-run tag.')
                return
            }

            dir(work_dir) {
                tarfile = sh(script: "find \"$work_dir/\" -name juju-core*.tar.gz", returnStdout: true).trim()
                print("Using build tarball: $tarfile")
            }
            xenial_ami = sh(
                script: "${scripts_dir}/get_ami.py xenial amd64 --virt hvm",
                returnStdout: true).trim()
            parallel(
                'Xenial': {
                    try {
                        withEnv(["JUJU_HOME=${cloud_city}"]){
                            sh("""
                            . $JUJU_HOME/juju-qa.jujuci && . $JUJU_HOME/ec2rc >2 /dev/null && \\
                            ${scripts_dir}/run-unit-tests c4.4xlarge $xenial_ami --local "$tarfile" --use-tmpfs --use-ppa ppa:juju/golang --force-archive
                            """)
                        }
                    } catch(e) {
                        error('Xenial test')
                    }
                },
            )
        }
    } finally {
        // Clean up after ourselves.
        dir(work_dir) {
            deleteDir()
        }
    }
}