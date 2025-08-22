# mic

A CLI tool to perform a bind mount from a source directory on the host to a target directory in the container. Linux-only.

## Usage
```
mic --target <dir> --source <source> --mount-namespace <path>
```

## Requirements
- Linux
- Rust (cargo)
- Root privileges (CAP_SYS_ADMIN)

## Build
```
cargo build --release
```

## Run
```
sudo ./target/release/mic --target /mnt/target --source /mnt/source --mount-namespace /proc/<pid>/ns/mnt
```
