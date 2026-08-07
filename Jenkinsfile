// Level2 Declarative Pipeline — Phase 1 CI skeleton.
// Paths match deploy/platform (Dockerfile) and web/.
// Phase 2: set ENABLE_PUSH / ENABLE_DEPLOY (main only). Do not store OPC secrets here.

pipeline {
  agent any

  options {
    timestamps()
    disableConcurrentBuilds()
    // Old Jenkins build records (logs/artifacts) — unrelated to Docker image GC, but keeps JENKINS_HOME bounded.
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
    // How many level2-collector:ci-<sha> tags to keep on the Docker host (ci-latest always kept).
    CI_IMAGE_KEEP = '5'
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
            bash -c '
              set -euo pipefail
              export PATH=/usr/local/go/bin:$PATH
              go run gotest.tools/gotestsum@v1.12.3 --junitfile test-results.xml -- ./... -coverprofile=cov.out -covermode=atomic
              go tool cover -html=cov.out -o coverage.html
            '
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
      recordCoverage(
        tools: [[parser: 'GO_COV', pattern: 'cov.out']],
        id: 'go-coverage',
        name: 'Go Coverage',
        sourceCodeRetention: 'NEVER'
      )
      publishHTML(target: [
        allowMissing: true,
        alwaysLinkToLastBuild: true,
        keepAll: true,
        reportDir: '.',
        reportFiles: 'coverage.html',
        reportName: 'Go Coverage HTML',
        reportTitles: 'Go Coverage'
      ])
      // Host Docker GC: only level2-collector:ci-<sha> tags. Does not touch
      // platform-collector / level2-jenkins / compose runtime images. Images still
      // referenced by a running container are skipped by docker rmi.
      sh '''
        set -euo pipefail
        KEEP="${CI_IMAGE_KEEP:-5}"
        REPO="${IMAGE_REPO:-level2-collector}"
        echo "Pruning ${REPO}:ci-* tags — keep newest ${KEEP} (plus ci-latest)"
        mapfile -t LINES < <(
          docker images --format '{{.CreatedAt}}\t{{.Repository}}:{{.Tag}}\t{{.ID}}' "${REPO}" 2>/dev/null \
            | awk -F'\t' -v p="${REPO}:ci-" '$2 ~ "^"p && $2 !~ /:ci-latest$/ {print}' \
            | sort -r
        )
        n=0
        for line in "${LINES[@]:-}"; do
          [ -n "$line" ] || continue
          n=$((n + 1))
          tag=$(printf '%s' "$line" | cut -f2)
          id=$(printf '%s' "$line" | cut -f3)
          if [ "$n" -le "$KEEP" ]; then
            echo "keep  ${tag} (${id})"
            continue
          fi
          echo "rm    ${tag} (${id})"
          docker rmi "$tag" 2>/dev/null || docker rmi "$id" 2>/dev/null || true
        done
        docker image prune -f >/dev/null || true
      '''
    }
  }
}
