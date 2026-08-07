// Level2 Declarative Pipeline — Phase 1 CI skeleton.
// Paths match deploy/platform (Dockerfile) and web/.
// Phase 2: set ENABLE_PUSH / ENABLE_DEPLOY (main only). Do not store OPC secrets here.

pipeline {
  agent any

  options {
    timestamps()
    disableConcurrentBuilds()
    buildDiscarder(logRotator(numToKeepStr: '20'))
  }

  parameters {
    booleanParam(name: 'ENABLE_PUSH', defaultValue: false, description: 'Phase 2: push image to registry')
    booleanParam(name: 'ENABLE_DEPLOY', defaultValue: false, description: 'Phase 2: deploy compose on lab VM (main only)')
  }

  environment {
    GOTOOLCHAIN = 'auto'
    IMAGE_NAME = 'level2-collector'
    // Override in Jenkins if you use a registry: e.g. registry.local/level2-collector
    IMAGE_REPO = "${IMAGE_NAME}"
  }

  stages {
    stage('Checkout') {
      steps {
        checkout scm
      }
    }

    stage('Go Test') {
      steps {
        sh '''
          docker run --rm \
            -e GOTOOLCHAIN=auto \
            -v "$PWD":/src -w /src \
            golang:1.24 \
            bash -lc 'go run gotest.tools/gotestsum@v1.12.3 --junitfile test-results.xml -- ./...'
        '''
      }
    }

    stage('Web Build') {
      steps {
        sh '''
          docker run --rm \
            -v "$PWD/web":/web -w /web \
            node:22-bookworm \
            bash -lc "npm ci && npm run build"
        '''
      }
    }

    stage('Docker Build') {
      steps {
        sh '''
          docker build \
            -f deploy/platform/Dockerfile \
            -t "${IMAGE_REPO}:ci-${GIT_COMMIT}" \
            -t "${IMAGE_REPO}:ci-latest" \
            .
        '''
      }
    }

    stage('Push') {
      when {
        allOf {
          expression { return params.ENABLE_PUSH }
          branch 'main'
        }
      }
      steps {
        echo 'Phase 2: configure registry credentials, then docker push ${IMAGE_REPO}:ci-${GIT_COMMIT}'
        // sh 'docker push "${IMAGE_REPO}:ci-${GIT_COMMIT}"'
      }
    }

    stage('Deploy') {
      when {
        allOf {
          expression { return params.ENABLE_DEPLOY }
          branch 'main'
        }
      }
      steps {
        echo 'Phase 2: deploy via deploy/platform/docker-compose.yml on the lab host (OPC stays in .env, not Jenkins).'
        // sh '''
        //   cd deploy/platform
        //   docker compose up -d --build
        // '''
      }
    }
  }

  post {
    always {
      echo "Finished ${env.GIT_COMMIT} on ${env.BRANCH_NAME}"
      junit allowEmptyResults: true, testResults: 'test-results.xml'
    }
  }
}
