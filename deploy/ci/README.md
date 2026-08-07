# Level2 CI/CD via Jenkins

Jenkins next to the lab stack (same VM / Docker host), configured with **JCasC** so bring-up is one command — no setup wizard.

**Status:** Phase 1 — CI checks + image build via Multibranch Pipeline. Deploy/push stages gated. No production secrets in-repo.

## One-command up

From the repo root on the lab VM (`~/level2`):

```bash
mkdir -p ~/jenkins_home
docker compose -f deploy/ci/docker-compose.yml --env-file deploy/ci/.env.example up -d --build
```

Lab defaults in `.env.example`: user `admin` / password `admin`. To customize, `cp deploy/ci/.env.example deploy/ci/.env`, edit, then use `--env-file deploy/ci/.env`.

`JENKINS_HOME` defaults to `/home/level2/jenkins_home` (override with `JENKINS_HOME_HOST` in `.env`).

UI: **http://\<vm-ip\>:8081/** (collector uses 8080).

Login (lab defaults from `.env.example`):

| Field | Default |
|-------|---------|
| User | `admin` |
| Password | `admin` |

Change `JENKINS_ADMIN_*` in `deploy/ci/.env` before first start (or recreate the `jenkins_home` volume after changing). Do not use lab passwords outside the VM.

Smoke check:

```bash
curl -sS -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8081/login
# expect 200
```

## What gets configured automatically

| Piece | How |
|-------|-----|
| Setup wizard | Disabled (`runSetupWizard=false`) |
| Plugins | `plugins.txt` via `jenkins-plugin-cli` in `Dockerfile.jenkins` |
| Admin user | JCasC + `JENKINS_ADMIN_ID` / `JENKINS_ADMIN_PASSWORD` |
| Multibranch job `level2` | Job DSL in `casc/jenkins.yaml` → repo `GITHUB_REPO_URL`, Script Path `Jenkinsfile` |
| Docker builds | Host `docker.sock` (sibling containers) |

Layout:

```
deploy/ci/
  docker-compose.yml
  Dockerfile.jenkins
  plugins.txt
  casc/jenkins.yaml      # JCasC + Job DSL seed
  .env.example           # lab secrets template (copy to .env)
  README.md
```

## Architecture

```mermaid
flowchart LR
  GH[GitHub\npopelev/level2] -->|poll SCM / webhook| J[Jenkins\ndeploy/ci compose]
  J --> A[Build agent\nsame Docker host]
  A -->|go test / npm build| T[Test results]
  A -->|docker build| IMG[collector image]
  IMG -.->|Phase 2: push| REG[Docker registry]
  IMG -.->|Phase 2: compose up| VM[level2-vm\ndeploy/platform]
```

## Docker builds without nested DinD

Mount the host Docker socket (`/var/run/docker.sock`). The controller runs **sibling** containers for:

- `golang:1.24` → `go test ./...`
- `node:22-bookworm` → `npm ci` / `npm run build` under `web/`
- `docker build -f deploy/platform/Dockerfile` → collector + embedded UI

`JENKINS_HOME` is bind-mounted to the **same path on the host and inside the container** (default `/home/level2/jenkins_home`). That way `docker run -v "$PWD":/src` from the Jenkinsfile resolves the workspace on the Docker host. A named volume alone breaks sibling mounts (empty `/src`, `go test` fails with “no main module”).

```bash
mkdir -p ~/jenkins_home
# optional override:
# echo 'JENKINS_HOME_HOST=/home/level2/jenkins_home' >> deploy/ci/.env
```

Compose runs the Jenkins container as `root` so it can use the host Docker socket.

Do **not** put Jenkins on the public internet; bind to LAN / VPN only.

## Pipeline stages

Root [`Jenkinsfile`](../../Jenkinsfile) (Declarative):

| Stage | Command / action | Paths |
|-------|------------------|--------|
| Checkout | SCM (Multibranch) | — |
| Go Test | `gotestsum --junitfile test-results.xml -- ./... -coverprofile=cov.out` + `go tool cover -html=cov.out -o coverage.html` in `golang:1.24` | `go.mod`, all packages → `test-results.xml`, `cov.out`, `coverage.html` |
| Web Build | `npm ci` && `npm run build` in `node:22-bookworm` | `web/` |
| Docker Build | `docker build -f deploy/platform/Dockerfile …` | platform Dockerfile |
| Push *(optional)* | gated by `ENABLE_PUSH=true` | Phase 2 |
| Deploy *(optional)* | gated by `ENABLE_DEPLOY=true` + `main` | Phase 2 |

After a build, from `job/level2` → branch → build:

| Report | Where in Jenkins UI |
|--------|---------------------|
| **Test Result** | Build page → **Test Result** (or job **Test Result** trend). From `test-results.xml` via `junit` (`allowEmptyResults: true`). |
| **Coverage** (trends) | Build page → **Coverage** / **Go Coverage** (Coverage plugin, `recordCoverage` on `cov.out` / `GO_COV`). Job-level trend chart appears after a few builds. |
| **Go Coverage HTML** | Build page (left) → **Go Coverage HTML** (HTML Publisher on `coverage.html`). |

Rebuild Jenkins after adding plugins (`docker compose … up -d --build`) so `junit`, `coverage`, and `htmlpublisher` are installed.

## GitHub credentials (optional)

`https://github.com/popelev/level2` is **public** — Multibranch seeds and scans without a PAT.

For a **private** fork, higher API limits, or private submodule access:

1. Create a fine-scoped PAT (contents: read).
2. Set in `deploy/ci/.env` (never commit):

   ```bash
   GITHUB_PAT=ghp_your_token_here
   GITHUB_CREDENTIALS_ID=github-pat
   ```

3. Recreate Jenkins (or reload CasC) so credential `github-pat` is updated:

   ```bash
   docker compose -f deploy/ci/docker-compose.yml --env-file deploy/ci/.env up -d --build
   ```

4. In Jenkins UI → job **level2** → Configure → Branch Sources → set Credentials to **github-pat** (secret text), or add `credentialsId('github-pat')` under the `git { … }` block in `casc/jenkins.yaml` and restart.

If the first Multibranch scan fails with auth errors and no PAT is set, Jenkins itself is still up — fix PAT / credentials, then **Scan Multibranch Pipeline Now**.

## Secrets

| Secret | Needed for CI? | How |
|--------|----------------|-----|
| Jenkins admin | Yes | `JENKINS_ADMIN_*` in `.env` (see `.env.example`) |
| GitHub PAT | Only if private / rate-limited | `GITHUB_PAT` → CasC credential `github-pat` |
| OPC UA / DB | **No** for unit CI | Stay in `deploy/platform/.env` |
| Registry / deploy | Phase 2 | Jenkins Credentials, never commit |

`.env` is gitignored (`**/.env`). Only `.env.example` is tracked.

## Triggers

1. **GitHub webhook** (preferred once reachable): Multibranch → GitHub hook.
2. **Periodic folder trigger** (CasC default: daily) + manual **Scan Multibranch Pipeline Now**.
3. Discover `main` + other branches; Script Path `Jenkinsfile`.

## Security notes

- Lab defaults (`admin`/`admin`) are for the VM only — change them.
- Socket mount grants Jenkins host-Docker power — treat the UI as highly privileged.
- Keep Phase 2 deploy behind parameters so PR builds cannot redeploy the lab.

## Phased rollout

### Phase 1 — CI only (current)

1. One-command compose up (above).
2. Open UI → job **level2** → scan branches → build `main`.
3. No auto-deploy; no registry push required.

### Phase 2 — Auto-deploy to lab VM

1. Registry or local tagging.
2. `ENABLE_DEPLOY` only on `main` after green CI.
3. Deploy via `deploy/platform/docker-compose.yml`.

## Sample pipeline reference

See repository root [`Jenkinsfile`](../../Jenkinsfile).
