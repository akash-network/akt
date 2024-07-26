# `akt`: Akash CLI

## Workflow

```sh
# create global account
akt account create main

# initialize environment
akt init .

akt deploy create foo \
    --template web --template-args "image=nginx" \
    --bid-selection interactive

akt deploy status foo
```

## Commands

### `akt init [dir]`

Initialize a local configuration directory.

Default directory at "$PWD/.akt"

### `akt account`

Manage accounts.

#### `akt account create <name>`
#### `akt account set-default <name>`
#### `akt account list`

### `akt network`
#### `akt network create`
#### `akt network list`

### `akt sdl`

#### `akt sdl create <name>`

|Argument|Required?|Description|
|---|---|---|
|`--template`|No|Use given template.|
|`--template-args`|No|Use given template arguments.|

#### `akt sdl list`

### `akt deploy`

#### `akt deploy create <name>`

|Argument|Required?|Description|
|---|---|---|
|`--account`|No|Use given account instead of default.|
|`--network`|No|Use given network instead of default.|
|`--template`|No|Use given template.|
|`--template-args`|No|Use given template arguments.|

#### `akt deploy status <name>`
#### `akt deploy destroy <name>`
#### `akt deploy list`

### `akt tx`

Escape hatch to low-level cosmos transactions.

### `akt query`

Escape hatch to low-level cosmos queries.


## Configuration

similar to git.

git remotes:  accounts, profiles, networks

working tree: deployments.

### Accounts

* `name`
* `type`
* `type-details` (directory, etc...)
* `address` for default network prefix (akash)?

### Networks

* `name`
* `chain-id`
* `address-prefix`
* `rpc`
* `grpc`

### Deployments

* `name`
* `dseq`
* `profile`
* `state`
* `sdl-path`

### Directory

1. if `--confdir` is set, use that.
1. `$PWD/.akt` (`--global`)
1. closest `.git` sibling named `.akt`
1. `~/.akt` (`--global`)

### Files

1. `config.yml`
1. `deployment/$name/sdl.yml`
1. `deployment/$name/state.yml`
