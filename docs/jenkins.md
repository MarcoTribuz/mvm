# Using mvm in Jenkins

mvm keeps several Meteor releases installed on an agent and picks the one each
build needs from the repository's `.meteor/release`. Builds running in parallel
on the same agent can use different releases: nothing global is switched.

The first build that needs a release downloads it (~600 MB, once); later builds
start immediately. Concurrent builds needing the same missing release download
it only once — the others wait on a lock.

## Install mvm on the agent

```sh
curl -fsSL https://raw.githubusercontent.com/marcotribuzio/mvm/main/install.sh | sh
# optional: pre-install the releases you use
mvm install 2.16 3.0.4 3.3
```

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

## Docker agents

Mount a named volume as `MVM_HOME` so releases survive the container:

```groovy
agent {
  docker {
    image 'ghcr.io/marcotribuzio/mvm:latest'
    args  '-v mvm-cache:/mvm -e MVM_HOME=/mvm -e MVM_AUTO_INSTALL=1'
  }
}
```

The volume must be writable by the container user (`chown` it once, or run
the container with a matching `--user`). Alternatively bake releases into your
image:

```dockerfile
FROM ghcr.io/marcotribuzio/mvm:latest
RUN mvm install 2.16 3.3
```

## Kubernetes agents

Use a `PersistentVolumeClaim` mounted at `/mvm` (`ReadWriteMany` if several
pods share it; mvm's file locks work on local disks and NFSv4). Set
`MVM_HOME=/mvm` and `MVM_AUTO_INSTALL=1` in the pod template.

## Air-gapped / internal mirror

Mirror the tarballs to Artifactory, Nexus or S3 keeping the layout
`<base>/<version>/meteor-bootstrap-os.linux.<arch>.tar.gz` and set:

```sh
export MVM_MIRROR=https://nexus.example.com/repository/meteor
```

## Housekeeping

```sh
mvm prune --unused-days 30     # e.g. from a weekly job
mvm doctor
```

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
