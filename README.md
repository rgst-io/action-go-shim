# action-go-shim

Shim for downloading binaries from Github Releases for use in Github
Actions. Specifically designed for Golang actions.

> [!NOTE]
> This is still a PoC, namely the composite action referenced has not
> yet been implemented.

## Usage

There are three methods of using this action.

### Pass-through

**Caveats**: Does not support inputs.

```yaml
steps:
- uses: rgst-io/action-go-shim@v0
  with:
    action_ref: <version>
    action_repo: your-org/your-repo
```

### Composite Actions

Create a [composite action]:

```yaml
# Due to a bug in Github Actions, you must set these inputs.
# See: https://github.com/actions/runner/issues/2473
inputs:
  action_ref:
    description: "[INTERNAL USAGE ONLY] `github.action_ref`"
    default: ${{ github.action_ref }}
  action_repo:
    description: "[INTERNAL USAGE ONLY] `github.action_repository`"
    default: ${{ github.action_repository }}

  # Your action's inputs go here
  # ...

runs:
  using: composite
  steps:
    - uses: rgst-io/action-go-shim@v0
      with:
        action_ref: ${{ inputs.action_ref }}
        action_repo: ${{ inputs.action_repo }}
```

### Binary

You can ship this action in Git with your action. This action shouldn't
change a lot, so it should be a minor amount of space used. Example:

1. Download **entire** release contents into `<repo root>/shim`.
2. Create an action consuming it

```yml
inputs:
  # Your action's inputs here
  # ...

runs:
  using: node24
  main: shim/shim.js
```

Configure the action by adding a `shim/shim-config.yml`.

## Configuration

Configuration is done three different ways based on how you've set up
the action.

### Methods

| Configuration Method   | Pass-through | Composite | Binary  |
| ---------------------- | ------------ | --------- | ------- |
| Github Actions inputs  | ✓           | ✓        | —      |
| Environment variables  | ✓           | ✓        | ✓      |
| YAML config file       | —           | —        | ✓      |

### Fields

| Field                  | Github Actions          | Environment Variable                   | YAML          |
|------------------------|-------------------------|----------------------------------------|---------------|
| CacheDirectory         | —                      | `ACTION_GO_SHIM_CACHE_DIR`             | —            |
| GithubToken            | `github_token`          | `GH_TOKEN`                             | —            |
| GithubActionRef        | `action_ref`            | `GITHUB_ACTION_REF`                    | `action_ref`  |
| GithubActionRepository | `action_repo`           | `GITHUB_ACTION_REPOSITORY`             | `action_repo` |
| Pattern                | `pattern`               | —                                     | `pattern`     |
| ValidateAttestations   | `validate_attestations` | `ACTION_GO_SHIM_VALIDATE_ATTESTATIONS` | —            |

## How it Works

This action takes the repo and ref portion of a Github Action "uses"
string (e.g., `rgst-io/stencil-action@v1` -> rgst-io/stencil-action, v1)
and downloads binaries from Github Releases.

This works by using `git ls-remote` on the repository and finding either
a direct tag match (e.g., `@v1.1.1`) or looking up the best match (`v1`
-> `v1.1.1`). In the case of commit (action pins), this will attempt to
find the **greatest** semantic version that has the same commit.
Branches cannot have Github Releases, so they are converted to a commit
and looked up the same way.

## License

LGPL-3.0

[composite action]: https://docs.github.com/en/actions/tutorials/create-actions/create-a-composite-action
