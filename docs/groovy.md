# Groovy / Jenkinsfile style

Covers `.groovy` files and `Jenkinsfile*` files in the corpus.  Groovy appears primarily in Jenkins pipeline scripts for DOI CI infrastructure (`doi/oi-janky-groovy`) and in per-repo `Jenkinsfile.*` files in `meta-scripts` and `tianon-dockerfiles`.

## Indentation

**Hard tabs, one per nesting level** — consistent with [universal.md](universal.md).

## Pipeline style: scripted, not declarative

All pipelines use the **scripted pipeline** form (`node { }`, `stage { }`) rather than the declarative form (`pipeline { agent { ... } }`).  No `pipeline { }` blocks appear anywhere in the corpus.

## Brace placement and closure chaining

Opening braces always go on the same line as the control structure or call.  When multiple wrappers nest immediately — because the inner wrapper is semantically inseparable from the outer — their opening braces chain on the same line and their closing braces pair at the end:

```groovy
for (repo in repos) { withEnv(['repo=' + repo]) {
    stage('Build') {
        sh '''...'''
    }
} }
```

```groovy
lock(label: 'repo-info-local', quantity: 1) { node('repo-info-local') {
    // ...
} }
```

Three tiers of how often chaining occurs in practice:

**Always chained** — `for` iteration immediately followed by `withEnv` is never split across lines; the loop variable needs to be injected before anything else can happen:
```groovy
for (def version in versions) { withEnv(['version=' + version]) {
    stage(version) {
        sh '...'
    }
} }
```

**Often chained** — `node(label)` immediately followed by another wrapper (`withEnv`, `dir`, etc.); `dir('...')` immediately followed by `stage('...')` or `deleteDir()`:
```groovy
dir('output') { stage('Archive') {
    archiveArtifacts '**'
} }
```

**Sometimes chained** — `withEnv([...])` followed by `stage(variant)` when iterating over variants; triple chains in loops (`for { stage { withEnv { } } }`).

Note: `configure { it / 'properties' << 'ClassName' { } }` is a separate pattern for Groovy XML builder manipulation of Jenkins job configuration — not a pipeline closure chain.

Corpus refs: [`doi/oi-janky-groovy/multiarch/target-pipeline.groovy#L77-L87`](https://github.com/docker-library/oi-janky-groovy/blob/2aafbd86f9e793de0145bc33bc865ad8b6d8e88a/multiarch/target-pipeline.groovy#L77-L87), [`doi/oi-janky-groovy/multiarch/generate-pipeline.groovy`](https://github.com/docker-library/oi-janky-groovy/blob/2aafbd86f9e793de0145bc33bc865ad8b6d8e88a/multiarch/generate-pipeline.groovy).

## String quoting

- **Single quotes** for simple string literals: `sh 'bashbrew fetch "$ACT_ON_IMAGE"'`
- **Double quotes** for Groovy string interpolation with `${}`: `"docker-hub-${env.BASHBREW_ARCH}"`
- **Triple single quotes** (`'''...'''`) for multi-line bash scripts — most common
- **Triple double quotes** (`"""..."""`) for multi-line strings requiring Groovy interpolation, primarily in generated JobDSL strings: `dsl += """..."""`

Corpus ref: [`doi/oi-janky-groovy/docs/generate-pipeline.groovy#L25`](https://github.com/docker-library/oi-janky-groovy/blob/2aafbd86f9e793de0145bc33bc865ad8b6d8e88a/docs/generate-pipeline.groovy#L25).

## Multi-line shell scripts

Multi-line bash passed to `sh` always begins with a shebang and `set` line:

```groovy
sh(returnStdout: true, script: '''#!/usr/bin/env bash
	set -Eeuo pipefail -x
	...
''')
```

The bash code inside triple-quoted strings is indented **one level deeper** than the `sh` call — the closing `'''` sits at the `sh`'s indentation level:

```groovy
stage('Build') {
	sh '''#!/usr/bin/env bash
		set -Eeuo pipefail -x
		bashbrew fetch "$ACT_ON_IMAGE"
	'''
}
```

### Suppressing trace for sensitive operations

When a block handles credentials or other sensitive values, `-x` is omitted from `set` with a comment, and `set +x` / `set -x` pairs isolate the sensitive commands:

```bash
set -Eeuo pipefail # no -x
docker login --username "$USERNAME" --password-stdin <<<"$PASSWORD"
```

Or inline within a script that is otherwise traced:

```bash
set +x
: "${INPUT_TOKEN:=$ACTIONS_RUNTIME_TOKEN}"
git config --local "http.$host/.extraheader" "Authorization: Basic $b64token"
set -x
```

This pattern appears in both Groovy shell blocks and standalone bash scripts where credential material would otherwise appear in `-x` trace output.

Corpus refs: [`meta-scripts/Jenkinsfile.deploy#L53`](https://github.com/docker-library/meta-scripts/blob/205031aee2fdfbbd449038afd58f0f0a6915c217/Jenkinsfile.deploy#L53), [`doi/oi-janky-groovy/docs/target-pipeline.groovy#L143`](https://github.com/docker-library/oi-janky-groovy/blob/2aafbd86f9e793de0145bc33bc865ad8b6d8e88a/docs/target-pipeline.groovy#L143), [`actions/checkout/checkout.sh#L47-L51`](https://github.com/tianon/actions/blob/c109aa98a82622edf55e0e6380a1672368930b30/checkout/checkout.sh#L47-L51).

Shell code within these blocks follows [bash.md](bash.md) conventions exactly.

## Multi-line parameter lists

When a function call spans multiple lines, each parameter goes on its own line with a **trailing comma** after the last one.  Inline `//` comments on positional arguments label what each one is:

```groovy
def vars = fileLoader.fromGit(
	'multiarch/vars.groovy',                                    // script
	'https://github.com/docker-library/oi-janky-groovy.git',   // repo
	'master',                                                   // branch
	null,                                                       // credentialsId
	'',                                                         // node/label
)
```

Corpus ref: [`doi/oi-janky-groovy/multiarch/target-pipeline.groovy#L4-L10`](https://github.com/docker-library/oi-janky-groovy/blob/2aafbd86f9e793de0145bc33bc865ad8b6d8e88a/multiarch/target-pipeline.groovy#L4-L10).

## Variable naming

- **`UPPERCASE`** for Jenkins environment variables accessed via `env.*`
- **`camelCase`** for local Groovy variables (`def buildEnvs`, `def children`)
- **Stage names** are title-case, kept to a single word when reasonably possible — the Jenkins pipeline view renders better with short names: `stage('Checkout')`, `stage('Meta')`, `stage('Deploy')`

## Comments

`//` for all single-line comments.  Block comments (`/* */`) do not appear.  Inline comments follow the same voice conventions as [prose.md](prose.md) — lowercase first word, no terminal period:

```groovy
quietPeriod: 15 * 60, // 15 minutes
cron('@daily'),        // check periodically, just in case
```

## Common wrappers

**`withCredentials([...])`** — list format with each credential type on its own line, trailing comma:

```groovy
withCredentials([
	usernamePassword(
		credentialsId: 'docker-hub-' + env.BASHBREW_ARCH,
		usernameVariable: 'DOCKER_USERNAME',
		passwordVariable: 'DOCKER_PASSWORD',
	),
]) {
	sh '''#!/usr/bin/env bash
		set -Eeuo pipefail # no -x
		...
	'''
}
```

**`dir()`** for changing the working directory.  **`deleteDir()`** for cleanup inside a `dir()` block.  **`sshagent([])`** for SSH credential injection.

## Shell output capture

`sh(returnStdout: true, script: '''...''').trim()` — `.trim()` is always chained to remove the trailing newline:

```groovy
env.TAGS = sh(returnStdout: true, script: '''#!/usr/bin/env bash
	set -Eeuo pipefail
	bashbrew cat --format '...' "$ACT_ON_IMAGE"
''').trim()
```

`.tokenize()` chains after `.trim()` when the output needs to be split into a list.

## Logging

`echo(...)` with parentheses for Jenkins console output — not `echo '...'`.

## Notable omissions

- Declarative pipeline syntax (`pipeline { }`, `agent { }`, `stages { }`) — always scripted
- `agent` block — always `node`
- `post { }` blocks — cleanup done inline
- `options { }` blocks — pipeline properties set via `properties([...])` calls inline
