# Scanning built images with Syft

TLDR: to scan images, we will mount their filesystem with `buildah mount`
and scan the filesystem with `syft scan dir:${mount_path} --override-default-catalogers image`.

The rest of this doc explains the reasoning and the consequences.

## Why we shouldn't scan images directly

Syft is able to scan container images directly using various providers, such as:

* `docker`: fetches the image from the docker daemon
  * uses an API that exports the uncompressed layers as a tarball
* `podman`: fetches the image from the podman service
  * uses an API compatible with the one used for docker
  * you can run the service with `systemctl --user start podman.socket` (comes with the podman RPM)
* `oci-archive`: scans a tarball exported with e.g. `buildah push ${image} oci-archive:image.tar`
  * the tarball contains compressed image layers
* `oci-dir`: scans a directory exported with e.g. `buildah push ${image} oci:image.dir/`
  * the directory contains compressed image layers

The problem is that every provider writes at least 1x the uncompressed image size into tmpdir.
Specifically:

| provider        | tmpdir content                                                          | tmpdir usage                                      |
| ---             | ---                                                                     | ---                                               |
| `docker`        | tarball with uncompressed layers + extracted tarball content            | 2x the uncompressed size                          |
| `podman`        | tarball with uncompressed layers + extracted tarball content            | 2x the uncompressed size                          |
| `oci-archive`   | extracted oci-archive content (compressed layers) + uncompressed layers | 1x the compressed size + 1x the uncompressed size |
| `oci-dir`       | uncompressed layers                                                     | 1x the uncompressed size                          |

For large images, this is a problem. It's not unusual to deal with 30Gi images in Konflux.
Writing 30Gi of data into /tmp takes up a significant portion of the scanning time
(details in [Numbers from testing in Konflux](#numbers-from-testing-in-konflux)).
It may even have negative effects on memory usage due to page cache,
despite /tmp not being a tmpfs in Kubernetes Pods.

When scanning the mounted filesystem as a `dir:`, syft doesn't write
any container layers into tmpdir (it doesn't even know it's scanning a container).
Syft may still write other cataloger-specific files, e.g. the RPM database,
but the tmpdir usage is orders of magnitude lower.

Once we've built the image, we already have all the uncompressed layers in the container storage.
Using `buildah mount` doesn't duplicate any data, just creates a view of the merged filesystem
using overlay mounts. So by scanning the mounted filesystem, we avoid duplicating any data.

<details>
<summary>Tmpdir usage investigation details, if you're interested</summary>

Uses an `inotifywait`-based script to watch file sizes as Syft writes them.
Save as `watch_dir_file_sizes.py`:

```python
#!/usr/bin/env python
import argparse
import subprocess
from pathlib import Path


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("watch_dir", type=Path)
    args = ap.parse_args()

    watch_dir: Path = args.watch_dir

    inotify_proc = subprocess.Popen(
        ["inotifywait", "-m", "-e", "modify,create", "-r", watch_dir],
        stdout=subprocess.PIPE,
        text=True,
    )
    assert inotify_proc.stdout is not None

    file_sizes: dict[Path, int] = {}

    try:
        while True:
            line: str = inotify_proc.stdout.readline()
            parts = line.split()
            if len(parts) != 3:
                continue

            # inotifywait output is {dir_path} {event_type} {filename}
            filepath = Path(parts[0], parts[2])
            if filepath.is_dir():
                continue

            size = filepath.stat().st_size
            prev_size = file_sizes.get(filepath, 0)
            file_sizes[filepath] = max(size, prev_size)
    except KeyboardInterrupt:
        pass

    inotify_proc.terminate()

    for path, size in file_sizes.items():
        print(size, path)


if __name__ == "__main__":
    main()
```

Set up the prerequisites:

```bash
podman pull quay.io/konflux-ci/task-runner:1.8.1
podman push quay.io/konflux-ci/task-runner:1.8.1 oci-archive:task-runner.tar
podman push quay.io/konflux-ci/task-runner:1.8.1 oci:task-runner.dir
systemctl --user start podman.socket
mkdir tmpdir
```

`docker` (the default image provider):

```bash
$ ./watch_dir_file_sizes.py tmpdir | numfmt --to=iec-i & \
  TMPDIR=$(realpath tmpdir) syft scan quay.io/konflux-ci/task-runner:1.8.1 >/dev/null && \
  pkill -INT --full watch_dir_file_sizes.py

1.3Gi tmpdir/stereoscope-1933529688/docker-daemon-image-3355960970/image.tar
84Mi tmpdir/stereoscope-1933529688/docker-tarball-image-1964494371/sha256:732bf689657b2e0a7c3fb830e3a881d1e3a01053aee58e3639bef800dd9259aa
1.2Gi tmpdir/stereoscope-1933529688/docker-tarball-image-1964494371/sha256:b48917e9311ddf200fdf096e79bbff8de0e20cc6d667a91744563f310a7fb65a
20Mi tmpdir/syft-cataloger-883043870/rpmdb-2347361859
```

`podman`:

```bash
$ ./watch_dir_file_sizes.py tmpdir | numfmt --to=iec-i & \
  TMPDIR=$(realpath tmpdir) syft scan podman:quay.io/konflux-ci/task-runner:1.8.1 >/dev/null && \
  pkill -INT --full watch_dir_file_sizes.py

1.3Gi tmpdir/stereoscope-4039701113/podman-daemon-image-1848484195/image.tar
84Mi tmpdir/stereoscope-4039701113/docker-tarball-image-511630454/sha256:732bf689657b2e0a7c3fb830e3a881d1e3a01053aee58e3639bef800dd9259aa
1.2Gi tmpdir/stereoscope-4039701113/docker-tarball-image-511630454/sha256:b48917e9311ddf200fdf096e79bbff8de0e20cc6d667a91744563f310a7fb65a
20Mi tmpdir/syft-cataloger-3608713446/rpmdb-3418269287
```

`oci-archive`:

```bash
$ ./watch_dir_file_sizes.py tmpdir | numfmt --to=iec-i & \
  TMPDIR=$(realpath tmpdir) syft scan oci-archive:task-runner.tar >/dev/null && \
  pkill -INT --full watch_dir_file_sizes.py

34Mi tmpdir/stereoscope-31600137/oci-tarball-image-1139133435/blobs/sha256/8b457fb1b26320aa35da6d429ea0efa5a81d9f904a24a8d0a4e1a1efcfd0e7b8
568 tmpdir/stereoscope-31600137/oci-tarball-image-1139133435/blobs/sha256/bf2c87e8b5cfce95f81f891258e83a6095e0ad16b9c955c1bfc51947c13ae014
411Mi tmpdir/stereoscope-31600137/oci-tarball-image-1139133435/blobs/sha256/d3cf76b0d4859d53ac23f825936c1bc3569dbf6268a304fe2f71391d2fbe7734
240 tmpdir/stereoscope-31600137/oci-tarball-image-1139133435/index.json
30 tmpdir/stereoscope-31600137/oci-tarball-image-1139133435/oci-layout
84Mi tmpdir/stereoscope-31600137/oci-dir-image-1493541447/sha256:732bf689657b2e0a7c3fb830e3a881d1e3a01053aee58e3639bef800dd9259aa
1.2Gi tmpdir/stereoscope-31600137/oci-dir-image-1493541447/sha256:b48917e9311ddf200fdf096e79bbff8de0e20cc6d667a91744563f310a7fb65a
20Mi tmpdir/syft-cataloger-1345145810/rpmdb-2297068399
```

`oci-dir`:

```bash
$ ./watch_dir_file_sizes.py tmpdir | numfmt --to=iec-i & \
  TMPDIR=$(realpath tmpdir) syft scan oci-dir:task-runner.dir >/dev/null && \
  pkill -INT --full watch_dir_file_sizes.py

84Mi tmpdir/stereoscope-1375041565/oci-dir-image-1003445472/sha256:732bf689657b2e0a7c3fb830e3a881d1e3a01053aee58e3639bef800dd9259aa
1.2Gi tmpdir/stereoscope-1375041565/oci-dir-image-1003445472/sha256:b48917e9311ddf200fdf096e79bbff8de0e20cc6d667a91744563f310a7fb65a
20Mi tmpdir/syft-cataloger-3516167678/rpmdb-4034107007
```

`dir`:

```bash
$ ./watch_dir_file_sizes.py tmpdir | numfmt --to=iec-i & \
  buildah unshare -- bash -c '
    container=$(buildah from quay.io/konflux-ci/task-runner:1.8.1)
    trap "buildah rm $container" EXIT
    rootfs=$(buildah mount $container)
    TMPDIR=$(realpath tmpdir) syft scan dir:$rootfs --override-default-catalogers=image >/dev/null
  ' && \
  pkill -INT --full watch_dir_file_sizes.py

20Mi tmpdir/syft-cataloger-837870144/rpmdb-1802446147
```

Out of curiosity, also verified that Syft doesn't clean up as it goes (only at the end),
so the tmpdir size does indeed grow to the sum of all file sizes by the end.

```bash
inotifywait -t 10 -m -e modify,create,delete -r tmpdir | while read -r line; do
  du --bytes tmpdir
done | awk '
  {if ($1 > sizes[$2]) sizes[$2] = $1}
  END {for (f in sizes) print sizes[f], f}
' | numfmt --to=iec-i &

TMPDIR=$(realpath tmpdir) syft scan ...
```

The maximum tmpdir size is as expected:

* `docker`, `podman`: 2.6Gi
* `oci-archive`: 1.8Gi
* `oci-dir`: 1.3Gi
* `dir`: 20Mi

</details>

### Numbers from testing in Konflux

The `dir:` and `oci-dir:` approaches were tested in Konflux using simple Tasks
that pull an image and run Syft on it. The test subject was an image with a 28Gi
uncompressed size (`quay.io/redhat-user-workloads/ramalama-tenant/rocm-tools:f522c7fabbf9e690fa708b67d53b153c12cd34d5-linux-d160-m4xlarge-amd64`).

Both approaches were tested with a 2Gi memory limit and a 4Gi limit to see if the
selected approach has an effect on memory usage.

| provider    | limit | runs | OOMKills | avg wall clock | avg memory | peak /tmp |
| ---         | ---   | ---  | ---      | ---            | ---        | ---       |
| dir         | 2Gi   | 4    | 1 (25%)  | 4:09           | 1,287 Mi   | 0 Mi      |
| dir         | 4Gi   | 4    | 0 (0%)   | 3:32           | 3,278 Mi   | 0 Mi      |
| oci-dir     | 2Gi   | 5    | 2 (40%)  | 12:53          | 1,330 Mi   | 28,852 Mi |
| oci-dir     | 4Gi   | 3    | 1 (33%)  | 8:32           | 3,964 Mi   | 28,852 Mi |

*Note: /tmp usage stats were collected in a polling loop, not using `inotify`,
so the 0Mi for overlay may not be 100% accurate.*

The main conclusive result is the wall clock time - `dir` is much faster than `oci-dir`.
And the /tmp usage of course, but we knew that already.

Memory usage results were less conclusive. Both approaches consume a similar amount of memory.
Both sometimes succeed even with a 2Gi limit, because Go's garbage collector
is aware of cgroup limits and runs more aggressively with a lower limit.
This is also why scanning was faster with a higher memory limit.

The `dir` approach does seem to consume less memory and get OOMKilled less often,
but the sample size is too small to make conclusions.
Claude proposes that writing 28Gi of data fills the available RAM with page cache.
It's unclear how much of an effect this really has, since the OS should clear page cache
whenever a program needs more memory.

Summary: `dir` is vastly better than any other approach when it comes to scan time and disk usage.
It *may* also be slightly better in terms of memory usage.

## Why we need --override-default-catalogers=image

Syft selects the list of catalogers based on what it's scanning.
By default, images have a different set of catalogers than directories.

You can get the list of catalogers like this:

```bash
syft cataloger list --override-default-catalogers image
syft cataloger list --override-default-catalogers directory
```

The `directory` tag has many catalogers that `image` does not, and vice versa.
For example:

* directory-only:
  * github-actions-usage-cataloger
  * go-module-file-cataloger
  * javascript-lock-cataloger
  * ...
* image-only:
  * javascript-package-cataloger
  * ruby-installed-gemspec-cataloger
  * ...

What this means is that by default, if we scan a container filesystem as a directory,
Syft will look for lockfiles, github workflows and other irrelevant files,
and will not look for installed JavaScript packages, Ruby packages and others.

The `--override-default-catalogers` option fixes that.

## Consequences of scanning the image as a directory

While the `--override-default-catalogers` fixes package discovery,
scanning a mounted filesystem instead of scanning an image still causes differences.
Some expected, some due to bugs in Syft.

### 1. The root package is useless

When scanning a container image the normal way, the root package in the SBOM has usable information:

<details>
<summary>root-package-image</summary>

```json
{
  "name": "quay.io/konflux-ci/task-runner",
  "SPDXID": "SPDXRef-DocumentRoot-Image-quay.io-konflux-ci-task-runner",
  "versionInfo": "1.8.1",
  "checksums": [
    {
      "algorithm": "SHA256",
      "checksumValue": "0ec5e8aa784db7724749a6c2bfb2c68be3dbb557ecc27824bedec037fe2bfd19"
    }
  ],
  "externalRefs": [
    {
      "referenceCategory": "PACKAGE-MANAGER",
      "referenceType": "purl",
      "referenceLocator": "pkg:oci/quay.io%2Fkonflux-ci%2Ftask-runner@sha256%3A0ec5e8aa784db7724749a6c2bfb2c68be3dbb557ecc27824bedec037fe2bfd19?arch=amd64&tag=1.8.1"
    }
  ],
  "primaryPackagePurpose": "CONTAINER"
}
```

</details>

The situation is quite a bit worse for scanning an oci-archive (what the Buildah task does today):

<details>
<summary>root-package-oci-archive</summary>

```json
{
  "name": "task-runner.tar",
  "SPDXID": "SPDXRef-DocumentRoot-Image-task-runner.tar",
  "versionInfo": "sha256:bf2c87e8b5cfce95f81f891258e83a6095e0ad16b9c955c1bfc51947c13ae014",
  "checksums": [
    {
      "algorithm": "SHA256",
      "checksumValue": "bf2c87e8b5cfce95f81f891258e83a6095e0ad16b9c955c1bfc51947c13ae014"
    }
  ],
  "externalRefs": [
    {
      "referenceCategory": "PACKAGE-MANAGER",
      "referenceType": "purl",
      "referenceLocator": "pkg:oci/task-runner.tar@sha256%3Abf2c87e8b5cfce95f81f891258e83a6095e0ad16b9c955c1bfc51947c13ae014?arch="
    }
  ],
  "primaryPackagePurpose": "CONTAINER"
}
```

</details>

And scanning the mounted file system makes it completely useless:

<details>
<summary>root-package-dir</summary>

```json
{
  "name": "/home/acmiel/.local/share/containers/storage/overlay/8daf0fc2f04dd4fc95bab852925819e153bddc2ee48dba5b02fdc697edcf7b60/merged",
  "SPDXID": "SPDXRef-DocumentRoot-Directory--home-acmiel-.local-share-containers-storage-overlay-8daf0fc2f04dd4fc95bab852925819e153bddc2ee48dba5b02fdc697edcf7b60-merged",
  "primaryPackagePurpose": "FILE"
}
```

</details>

The situation is similar for the Cyclonedx `.metadata.component`.

None of this really matters. The Syft SBOMs go through Mobster to become the final SBOM.
Mobster always replaces the root package with one that represents the image
([CycloneDX][mobster-rootpkg-cdx], [SPDX][mobster-rootpkg-spdx]).

### 2. We lose the layerID property

Syft doesn't report just packages, but also the individual files in those packages.
When scanning a container image, Syft reports the layer ID where each file was found:

```json
{
  "fileName": "etc/GREP_COLORS",
  "comment": "layerID: sha256:732bf689657b2e0a7c3fb830e3a881d1e3a01053aee58e3639bef800dd9259aa"
}
```

When scanning a directory, Syft doesn't report this of course.

We don't really care. In fact, Mobster [drops the `.files` array][mobster-drop-files] altogether.

### 3. Scanning a dir reports more files, changes packageVerificationCode

Due to [anchore/syft#5019], when packages include hardlinked files,
Syft only reports them correctly when scanning a directory.
This also changes the `packageVerificationCodeValue` for the affected packages.

This consequence is actually a bugfix, but does have a minor negative effect on Mobster.

When doing contextualization, Mobster matches packages from the current SBOM to the base image SBOM.
One of the attributes Mobster considers in this process is `packageVerificationCodeValue`.
In practical terms: when generating an SBOM for `konflux-build-cli` using this new approach,
the contextualization misses 5 RPM packages that it normally would have caught.
The SBOM reports they belong to the `konflux-build-cli` image, but they belong to `task-runner`.

This problem will only affect builds where the base image SBOM was generated the old way.
Over time, as base images rebuild with the new approach, the problem will correct itself.

[mobster-rootpkg-cdx]: https://github.com/konflux-ci/mobster/blob/ef5f0a4613c8e77d70ba823b9b24f9c105bbb6b6/src/mobster/cmd/generate/oci_image/add_image.py#L48
[mobster-rootpkg-spdx]: https://github.com/konflux-ci/mobster/blob/ef5f0a4613c8e77d70ba823b9b24f9c105bbb6b6/src/mobster/cmd/generate/oci_image/spdx_utils.py#L326-L336
[mobster-drop-files]: https://github.com/konflux-ci/mobster/blob/ef5f0a4613c8e77d70ba823b9b24f9c105bbb6b6/src/mobster/sbom/merge.py#L583-L585
[anchore/syft#5019]: https://github.com/anchore/syft/issues/5019
