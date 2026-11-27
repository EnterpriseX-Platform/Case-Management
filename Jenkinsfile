// Build, test and deploy to QA. Promote to UAT only after QA passes
// (run again with DEPLOY_ENV=uat). First time per environment, apply
// deploy/k8s/case-management.yaml into the namespace.
pipeline {
  agent any
  parameters {
    string(name: 'GIT_REF', defaultValue: 'main', description: 'Branch / tag to build')
    string(name: 'REGISTRY', defaultValue: '', description: 'Image registry, e.g. registry.example.com/cm')
    choice(name: 'DEPLOY_ENV', choices: ['qa', 'uat', 'none'], description: 'Target environment')
    string(name: 'K8S_NAMESPACE', defaultValue: 'case-management', description: 'Kubernetes namespace')
  }
  environment {
    TAG = "${params.GIT_REF}-${env.BUILD_NUMBER}"
  }
  stages {
    stage('Checkout') {
      steps { checkout scmGit(branches: [[name: params.GIT_REF]], userRemoteConfigs: scm.userRemoteConfigs) }
    }
    stage('Test backend') {
      steps {
        sh '''
          docker rm -f cm-ci-pg-$BUILD_NUMBER 2>/dev/null || true
          docker run -d --name cm-ci-pg-$BUILD_NUMBER -e POSTGRES_DB=cm -e POSTGRES_USER=cm -e POSTGRES_PASSWORD=ci -p 0:5432 postgres:16-alpine
          PORT=$(docker port cm-ci-pg-$BUILD_NUMBER 5432/tcp | head -1 | cut -d: -f2)
          for i in $(seq 1 30); do docker exec cm-ci-pg-$BUILD_NUMBER pg_isready -U cm && break; sleep 1; done
          cd backend
          go vet ./...
          go test ./... -count=1
          CM_TEST_DATABASE_URL="postgres://cm:ci@127.0.0.1:$PORT/cm?sslmode=disable" go test ./internal/... -count=1
        '''
      }
      post { always { sh 'docker rm -f cm-ci-pg-$BUILD_NUMBER || true' } }
    }
    stage('Build frontend') {
      steps { dir('frontend') { sh 'npm ci --no-audit --no-fund && npm run build' } }
    }
    stage('Images') {
      when { expression { params.REGISTRY?.trim() } }
      steps {
        sh '''
          docker build -t $REGISTRY/cm-api:$TAG backend
          docker build --build-arg API_URL=http://cm-api:8080 -t $REGISTRY/cm-web:$TAG frontend
          docker push $REGISTRY/cm-api:$TAG
          docker push $REGISTRY/cm-web:$TAG
        '''
      }
    }
    stage('Deploy') {
      when { expression { params.DEPLOY_ENV != 'none' && params.REGISTRY?.trim() } }
      steps {
        sh '''
          NS=$K8S_NAMESPACE-$DEPLOY_ENV
          kubectl -n $NS set image deployment/cm-api api=$REGISTRY/cm-api:$TAG
          for svc in engine integration scheduler notify; do
            kubectl -n $NS set image deployment/cm-$svc $svc=$REGISTRY/cm-api:$TAG
          done
          kubectl -n $NS set image deployment/cm-web web=$REGISTRY/cm-web:$TAG
          kubectl -n $NS rollout status deployment/cm-api --timeout=180s
          for svc in engine integration scheduler notify; do
            kubectl -n $NS rollout status deployment/cm-$svc --timeout=180s
          done
          kubectl -n $NS rollout status deployment/cm-web --timeout=180s
        '''
      }
    }
  }
}
