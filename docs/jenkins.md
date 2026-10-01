# Using mvm in Jenkins

mvm keeps several Meteor releases installed on an agent and picks the one each
build needs from the repository's `.meteor/release`. Builds running in parallel
on the same agent can use different releases: nothing global is switched.

The first build that needs a release downloads it (~600 MB, once); later builds
start immediately. Concurrent builds needing the same missing release download
it only once — the others wait on a lock.

## Install mvm on the agent

```sh
curl -fsSL https://raw.githubusercontent.com/MarcoTribuz/mvm/main/install.sh | PROFILE=/dev/null sh
# optional: pre-install the releases you use
~/.local/bin/mvm install 2.16 3.0.4 3.3
```

Jenkins `sh` steps do not read shell profiles, so `PROFILE=/dev/null` skips
that part of the installer; put `~/.local/bin` (and `~/.mvm/bin` for the shim)
on `PATH` in the node configuration or the pipeline's `environment` instead.
To pin the mvm version on every agent, set `MVM_VERSION=0.3.0` for the
installer.

## Static agents (VMs)

`MVM_HOME` defaults to `~/.mvm` of the Jenkins user, which persists between
builds.

```groovy
pipeline {
  agent { label 'linux' }
  environment { MVM_AUTO_INSTALL = '1' }
  stages {
    stage('Deps')  { steps { sh 'mvm exec -- meteor npm ci' } }
    stage('Test')  { steps { sh 'mvm exec -- meteor npm test' } }
    stage('Build') { steps { sh 'mvm exec -- meteor build ../out --server-only --architecture os.linux.x86_64' } }
  }
}
```

`mvm exec` also puts the node/npm bundled with that Meteor release on `PATH`,
so plain `npm ci` works too and always matches the release (Node 14 for 2.x,
Node 20/22 for 3.x) regardless of what is installed on the agent.

### Transparent `meteor` command

If you prefer existing scripts to keep calling `meteor` directly:

```sh
mvm init          # creates $MVM_HOME/bin/meteor and prints the PATH line
```

```groovy
environment {
  PATH = "${env.HOME}/.mvm/bin:${env.PATH}"
  MVM_AUTO_INSTALL = '1'
}
steps { sh 'meteor build ../out --server-only' }
```

## Aliases

Aliases are stored in `MVM_HOME`, so they are per agent (or per volume). They
let you change the release used by many jobs in one place, e.g. for jobs and
tools that have no `.meteor/release`:

```sh
mvm alias ci-tools 3.3           # once per agent, e.g. from a setup job
```

```groovy
environment { MVM_METEOR_VERSION = 'ci-tools' }   // or `mvm exec ci-tools -- …`
```

A repository's `.meteor/release` always wins over an alias. Avoid the built-in
`latest` in pipelines: it moves when Meteor publishes a release, so builds stop
being reproducible. It also asks the GitHub API (cached for an hour), so set
`GITHUB_TOKEN` on busy agents and do not use it on air-gapped ones.

## Docker agents

Mount a named volume as `MVM_HOME` so releases survive the container:

```groovy
agent {
  docker {
    image 'ghcr.io/marcotribuz/mvm:latest'
    args  '-v mvm-cache:/mvm -e MVM_HOME=/mvm -e MVM_AUTO_INSTALL=1'
  }
}
```

The volume must be writable by the container user (`chown` it once, or run
the container with a matching `--user`). Alternatively bake releases into your
image:

```dockerfile
FROM ghcr.io/marcotribuz/mvm:latest
RUN mvm install 2.16 3.3
```

## Kubernetes agents

Use a `PersistentVolumeClaim` mounted at `/mvm` (`ReadWriteMany` if several
pods share it; mvm's file locks work on local disks and NFSv4). Set
`MVM_HOME=/mvm` and `MVM_AUTO_INSTALL=1` in the pod template.

## Air-gapped / internal mirror

Mirror the tarballs to Artifactory, Nexus or S3 keeping the layout
`<base>/<version>/meteor-bootstrap-os.<os>.<arch>.tar.gz` and set:

```sh
export MVM_MIRROR=https://nexus.example.com/repository/meteor
```

## Housekeeping

```sh
mvm prune --unused-days 30     # e.g. from a weekly job
mvm doctor
```

`prune` never removes the default release, but it does remove releases that
only an alias points to, so pair it with `--keep` or re-run `mvm install <alias>`
in the job that uses the alias (it is a no-op when already installed).

### Updating mvm

On static agents, update the binary from the same weekly job:

```sh
mvm self-update 0.3.0          # pin a version; plain `mvm self-update` takes the latest
```

It downloads from GitHub releases and checks the release's `checksums.txt`
before replacing the binary; the Jenkins user needs write access to its
directory (`~/.local/bin` by default). Builds already running keep the old
binary. On Docker and Kubernetes agents, bump the image tag
(`ghcr.io/marcotribuz/mvm:<version>`) instead: changes inside a container are
lost. Air-gapped agents cannot self-update; ship the binary like any other tool.

## Shared library helper (optional)

```groovy
// vars/withMeteor.groovy
def call(Closure body) {
  withEnv(['MVM_AUTO_INSTALL=1', "PATH=${env.HOME}/.mvm/bin:${env.PATH}"]) {
    sh 'mvm current'
    body()
  }
}
```

```groovy
withMeteor { sh 'meteor build ../out --server-only' }
```
