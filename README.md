## mic — fsconfig-based mounting helper

### Overview

Small Go utility demonstrating the Linux `fsconfig` / `fsopen` / `fsmount` / `move_mount` APIs.
The program creates a filesystem context, applies fsconfig options, mounts the filesystem and
optionally moves the mount into a different mount namespace.

## Features

- Use modern `fsconfig` / `fsopen` / `fsmount` syscalls to create mounts.
- Accept repeated `-o key=val` options (flags or key=val strings).
- Optionally move the resulting mount into another mount namespace via `-mount_namespace`.
- Unit tests exercise argument parsing and option handling; an optional integration test performs a real mount.

## Files

- `main.go` — program implementation.
- `main_test.go` — unit tests for argument parsing and a skipped-by-default integration test for real mounts.

## Build

```bash
cd mic
go build -o mic
```

## Usage

```bash
# Mount tmpfs at /mnt/mytarget with size 64M (requires root/capabilities)
sudo ./mic -target /mnt/mytarget -source none -fstype tmpfs -o size=64M

# Example using a source device/path (like mount(8) source)
sudo ./mic -target /mnt/mytarget -source /dev/sdb1 -fstype ext4 -o ro

# Move the mount into an existing mount namespace (e.g. /proc/<pid>/ns/mnt)
sudo ./mic -target /path/in/target/ns -mount_namespace /proc/<pid>/ns/mnt -fstype tmpfs -o size=64M
```

## Notes and requirements

- Privileges: mounting and namespace operations require CAP_SYS_ADMIN (usually root). Run the binary with `sudo` or give the process the necessary capabilities.
- Kernel support: The program uses relatively new syscalls (`fsopen`, `fsconfig`, `fsmount`, `move_mount`, `setns`). The running kernel must support these; otherwise the syscalls will fail and the program will report errors.
- Namespace semantics: `-mount_namespace` causes the program to `setns` into the target namespace briefly to ensure the mount path exists there and then performs the `move_mount` to attach the mount inside that namespace. The program uses `runtime.LockOSThread()` to confine `setns` to a single OS thread.
- Portability: intentionally Linux-only (`//go:build linux`).

## Testing

Unit tests are fast and do not perform privileged operations:

```bash
# Run unit tests (fast)
cd mic
go test ./...
```

### Integration mount test (skipped by default)

There is an integration-style test in `main_test.go` named `TestMountOperation` that performs a real mount. It is skipped unless you explicitly enable it. To run it:

```bash
# Run as root and enable the mount test
cd mic
sudo env RUN_MOUNT_TESTS=1 go test -v ./... -run TestMountOperation
```

This test will:
- Build the package to a temporary binary.
- Run the binary to mount a `tmpfs` at a temporary directory.
- Verify the mount exists in `/proc/mounts`.
- Unmount and clean up.

## Caveats and troubleshooting

- If `move_mount` or `setns` return EPERM/EXDEV/ENOSYS, check kernel version and capabilities. Some distributions or kernels may not support moving mounts between namespaces or the `fsconfig` API.
- If you need a more robust cross-namespace attach behavior, consider running a small helper process inside the target namespace to perform the attach (this repository can be extended to provide such a helper).

## Contributions and next steps

- Add a syscall abstraction to allow unit-testing of failure paths.
- Implement an optional helper to perform cross-namespace attach from within the target namespace (safer across kernel versions).

## License

See `LICENSE` in the repository root.
