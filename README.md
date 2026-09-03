# pulumi-provider-multirotate

A Pulumi resource provider that rotates a cursor over a fixed number of
timestamps on a time basis, for periodic credential rotation.

A `MultiRotate` resource keeps `count` timestamps and an `index` into them.
When the timestamp at `index` becomes older than `rotationPeriodDays`, the
cursor advances to the next slot and that slot is re-stamped with the current
time. Deriving a secret from `currentTimestamp` therefore gives you a secret
that changes every `rotationPeriodDays`, while the previous `count - 1` values
remain visible in `timestamps` so old credentials can be kept alive during an
overlap window.

## Using it from TypeScript

This repository is private, so both halves of the install rely on your existing
GitHub credentials:

- the **SDK** is installed by pnpm straight from this repo over git,
- the **provider binary** is downloaded by Pulumi from this repo's GitHub
  releases, which requires a `GITHUB_TOKEN` in the environment.

### 1. Add the SDK

```sh
pnpm add "github:Extrality/pulumi-provider-multirotate#v0.1.0&path:/sdk/nodejs"
```

`sdk/nodejs` is committed pre-compiled, so there is no build step at install
time. Pin a tag (`#v0.1.0`) rather than a branch: the tag guarantees the SDK
matches the provider binary published under the same version.

`#semver:^0.1.0&path:/sdk/nodejs` also works if you would rather track a range.

### 2. Give Pulumi access to the release assets

The generated SDK declares
`pluginDownloadURL: github://api.github.com/Extrality/pulumi-provider-multirotate`,
so `pulumi up` fetches the matching `pulumi-resource-multirotate` binary from
this repo's releases automatically. Because the repo is private, export a token
with `repo` read access wherever you run Pulumi (your shell, CI, Pulumi
Deployments):

```sh
export GITHUB_TOKEN="$(gh auth token)"
```

### 3. Use it

```ts
import * as multirotate from "@extrality/pulumi-multirotate";

const rotation = new multirotate.MultiRotate("db-password", {
    count: 2,              // keep the current and the previous secret alive
    rotationPeriodDays: 30,
});

// Derive whatever you need from the rotating timestamps.
export const currentTimestamp = rotation.currentTimestamp;
export const allTimestamps = rotation.timestamps;
export const activeSlot = rotation.index;
```

`pulumi up` is a no-op until the timestamp at the cursor expires, at which point
it advances the cursor and re-stamps that slot. Run `pulumi up` on a schedule to
make rotation actually happen.

### Inputs

| Name                 | Type     | Default | Description                                          |
| -------------------- | -------- | ------- | ---------------------------------------------------- |
| `count`              | `number` | `1`     | Number of timestamps in the rotation window.         |
| `rotationPeriodDays` | `number` | `60`    | Days a timestamp stays valid before it is rotated.   |

Both must be positive integers.

### Outputs

| Name               | Type       | Description                                    |
| ------------------ | ---------- | ---------------------------------------------- |
| `index`            | `number`   | Index of the last rotated timestamp.           |
| `timestamps`       | `string[]` | The rotation window, ISO-8601, UTC.            |
| `currentTimestamp` | `string`   | `timestamps[index]`.                           |

## Developing

```sh
make build          # build ./bin/pulumi-resource-multirotate
make test           # go test -race ./...
make lint           # go vet + gofmt check
make schema         # print the schema the built plugin serves
make sdk            # regenerate + compile sdk/nodejs
make install_plugin # install the local build into the Pulumi plugin cache
```

`make install_plugin` followed by pointing a test program at `sdk/nodejs` via
`pnpm add ./path/to/sdk/nodejs` is the quickest way to try a change end to end.

### Cutting a release

1. Run the **release-prepare** workflow from the Actions tab with a version like
   `0.1.0`. It runs the tests, regenerates `sdk/nodejs` pinned to that version,
   and opens a `release/v0.1.0` pull request.
2. In that PR, click **Approve workflows to run** in the merge box.
3. Review and merge it. The diff should be two lines in
   `sdk/nodejs/package.json` — nothing else in the generated SDK carries the
   version.
4. Merging triggers **release-publish**, which tags the merge commit `v0.1.0`,
   builds the provider, and attaches the archives to the GitHub release.

The Pulumi CLI version is pinned in both workflows and in `ci.yml`, because
`sdk_check` compares generated output byte for byte. Bumping it means
regenerating `sdk/` in the same PR.

Release assets are named
`pulumi-resource-multirotate-v<version>-<os>-<arch>.tar.gz` containing the bare
binary, which is the layout Pulumi's `github://` plugin downloader expects.

## License

MIT. See [LICENSE](./LICENSE).
