# action-go-shim

Shim for downloading binaries from Github Releases for use in Github
Actions. Specifically designed for Golang actions.

> [!NOTE]
> This is still a PoC, namely the composite action referenced has not
> yet been implemented.

## Usage

Create a [composite action] that uses this action:

```yaml
# Due to a bug in Github Actions, you must set these inputs.
# See: https://github.com/actions/runner/issues/2473
inputs:
  action_ref:
    default: ${{ github.action_ref }}
  action_repo:
    default: ${{ github.action_repository}}

runs:
  using: composite
  steps:
    - uses: rgst-io/action-go-shim@v0
      with:
        action_ref: ${{ inputs.action_ref }}
        action_repo: ${{ inputs.action_repo }}
```

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

## Local Development

**TODO**: This section will address how to develop this action and
consuming actions locally, also with a stencil module for creating one.

## License

LGPL-3.0

[composite action]: https://docs.github.com/en/actions/tutorials/create-actions/create-a-composite-action
