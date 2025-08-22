use clap::Parser;
use nix::sched::{setns, CloneFlags};
use rustix::mount::{move_mount, open_tree, MoveMountFlags, OpenTreeFlags};
use std::os::fd::AsFd;
// use rustix::process::{setns, Namespace};
use std::fs::File;
use std::os::unix::fs::PermissionsExt;
use std::path::Path;
use std::process;

#[derive(Parser)]
#[command(author, version, about)]
struct Args {
    /// Target mountpoint directory
    #[arg(long)]
    target: String,
    /// Source device or path
    #[arg(long)]
    source: String,
    /// Path to target mount namespace
    #[arg(long)]
    mount_namespace: String,
}

fn main() {
    let args = Args::parse();

    // Ensure target exists and is a directory
    let target = Path::new(&args.target);
    if !target.exists() || !target.is_dir() {
        eprintln!(
            "target does not exist or is not a directory: {}",
            args.target
        );
        process::exit(1);
    }
    // Ensure source exists and is a directory
    let source = Path::new(&args.source);
    if !source.exists() || !source.is_dir() {
        eprintln!(
            "source does not exist or is not a directory: {}",
            args.source
        );
        process::exit(1);
    }
    let source_fd = match open_tree(
        rustix::fs::CWD,
        source,
        OpenTreeFlags::OPEN_TREE_CLONE | OpenTreeFlags::AT_RECURSIVE,
    ) {
        Ok(fd) => fd,
        Err(e) => {
            eprintln!("open source {} failed: {}", args.source, e);
            process::exit(1);
        }
    };
    let orig_ns = match File::open("/proc/self/ns/mnt") {
        Ok(f) => f,
        Err(e) => {
            eprintln!("open original mount namespace failed: {}", e);
            process::exit(1);
        }
    };
    // Optionally setns into mount namespace
    // Mount namespace switching using nix::setns
    if !args.mount_namespace.is_empty() {
        let ns_file = match File::open(&args.mount_namespace) {
            Ok(f) => f,
            Err(e) => {
                eprintln!(
                    "open mount namespace {} failed: {}",
                    args.mount_namespace, e
                );
                process::exit(1);
            }
        };
        // CLONE_NEWNS is 0x00020000
        if let Err(e) = setns(&ns_file, CloneFlags::CLONE_NEWNS) {
            eprintln!("setns to {} failed: {}", args.mount_namespace, e);
            process::exit(1);
        }
    }

    // Create the target directory with permission 755 before move_mount
    if let Err(e) = std::fs::create_dir_all(target) {
        eprintln!("failed to create target directory {}: {}", args.target, e);
        process::exit(1);
    }

    if let Err(e) = std::fs::set_permissions(target, std::fs::Permissions::from_mode(0o755)) {
        eprintln!(
            "failed to set permissions on target directory {}: {}",
            args.target, e
        );
        process::exit(1);
    }

    if let Err(e) = move_mount(
        source_fd.as_fd(),
        "",
        rustix::fs::CWD,
        target,
        MoveMountFlags::MOVE_MOUNT_F_EMPTY_PATH,
    ) {
        eprintln!("move_mount failed: {}", e);
        process::exit(1);
    }
    // restore original namespace
    if let Err(e) = setns(&orig_ns, CloneFlags::CLONE_NEWNS) {
        eprintln!("setns back to original namespace failed: {}", e);
        process::exit(1);
    }
}
