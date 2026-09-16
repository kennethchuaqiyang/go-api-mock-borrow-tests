pipeline {
    agent {
        docker {
            image 'golang:1.22'
        }
    }

    stages {
        stage('Install test tooling') {
            steps {
                // gotestsum wraps `go test` and can emit a JUnit XML report,
                // which Jenkins can use for a proper pass/fail trend view.
                sh 'go install gotest.tools/gotestsum@latest'
            }
        }

        stage('Run API tests') {
            steps {
                sh '''
                    export PATH=$PATH:$(go env GOPATH)/bin
                    gotestsum --junitfile results.xml --format standard-verbose -- ./...
                '''
            }
        }
    }

    post {
        always {
            junit 'results.xml'
        }
    }
}