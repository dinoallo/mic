//go:build linux

package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Minimal set of fsconfig/move_mount constants from Linux kernel headers.
const (
	FSCONFIG_SET_FLAG        = 0
	FSCONFIG_SET_STRING      = 1
	FSCONFIG_SET_BINARY      = 2
	FSCONFIG_SET_PATH        = 3
	FSCONFIG_SET_PATH_EMPTY  = 4
	FSCONFIG_CMD_CREATE      = 5
	FSCONFIG_CMD_RECONFIGURE = 6
)

const (
	// move_mount flags
	MOVE_MOUNT_F_EMPTY_PATH = 0x00000004
)

func bs(s string) *byte {
	p, _ := syscall.BytePtrFromString(s)
	return p
}

func fsopen(fsType string, flags uint) (int, error) {
	// syscall: int fsopen(const char *fs_name, unsigned int flags);
	ptr := uintptr(unsafe.Pointer(bs(fsType)))
	r1, _, err := unix.Syscall(unix.SYS_FSOPEN, ptr, uintptr(flags), 0)
	if err != 0 {
		return -1, err
	}
	return int(r1), nil
}

func fsconfig(fd int, cmd uint, key, value *byte, aux int) error {
	// syscall: int fsconfig(int fs_fd, unsigned int cmd, const char *key,
	//                        const void *value, int aux);
	kptr := uintptr(0)
	vptr := uintptr(0)
	if key != nil {
		kptr = uintptr(unsafe.Pointer(key))
	}
	if value != nil {
		vptr = uintptr(unsafe.Pointer(value))
	}
	_, _, err := unix.Syscall6(unix.SYS_FSCONFIG, uintptr(fd), uintptr(cmd), kptr, vptr, uintptr(aux), 0)
	if err != 0 {
		return err
	}
	return nil
}

func fsmount(fd int, flags uint, attr_flags uint) (int, error) {
	// syscall: int fsmount(int fs_fd, unsigned int flags, unsigned int attr_flags);
	r1, _, err := unix.Syscall(unix.SYS_FSMOUNT, uintptr(fd), uintptr(flags), uintptr(attr_flags))
	if err != 0 {
		return -1, err
	}
	return int(r1), nil
}

func moveMount(fromFd int, fromPath string, toFd int, toPath string, flags uint) error {
	// syscall: int move_mount(int from_dfd, const char *from_pathname,
	//                         int to_dfd, const char *to_pathname, unsigned int flags);
	fromPathPtr := uintptr(0)
	toPathPtr := uintptr(0)
	if fromPath != "" {
		fromPathPtr = uintptr(unsafe.Pointer(bs(fromPath)))
	}
	if toPath != "" {
		toPathPtr = uintptr(unsafe.Pointer(bs(toPath)))
	}
	_, _, err := unix.Syscall6(unix.SYS_MOVE_MOUNT, uintptr(fromFd), fromPathPtr, uintptr(toFd), toPathPtr, uintptr(flags), 0)
	if err != 0 {
		return err
	}
	return nil
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: %s -target <dir> [-source <source>] [-fstype <type>] [-mount_namespace <path>] [-o key=val]...\n", os.Args[0])
	flag.PrintDefaults()
}

// parseArgs parses command-line arguments passed as a slice (excluding argv[0])
// and returns the target, fstype, options and an error when parsing fails or
// when the required -target is missing. This is testable without performing
// any privileged syscalls.
func parseArgs(args []string) (target string, fstype string, mountNS string, source string, opts []string, err error) {
	fs := flag.NewFlagSet("mic", flag.ContinueOnError)
	var o multiString
	fs.StringVar(&target, "target", "", "Target mountpoint directory")
	fs.StringVar(&fstype, "fstype", "tmpfs", "Filesystem type to mount (e.g. tmpfs)")
	fs.StringVar(&mountNS, "mount_namespace", "", "Path to target mount namespace (e.g. /proc/<pid>/ns/mnt)")
	fs.StringVar(&source, "source", "", "Source device or path (like mount(8) source)")
	fs.Var(&o, "o", "fsconfig option as key=val; can be repeated")
	// Silence default output on parse errors; caller can inspect err
	if err := fs.Parse(args); err != nil {
		return "", "", "", "", nil, err
	}
	if target == "" {
		return "", "", "", "", nil, fmt.Errorf("missing -target")
	}
	return target, fstype, mountNS, source, []string(o), nil
}

func main() {
	target, fstype, mountNS, source, opts, err := parseArgs(os.Args[1:])
	if err != nil {
		usage()
		os.Exit(2)
	}

	// ensure target exists
	st, err := os.Stat(target)
	if err != nil {
		fmt.Fprintf(os.Stderr, "target error: %v\n", err)
		os.Exit(1)
	}
	if !st.IsDir() {
		fmt.Fprintf(os.Stderr, "target is not a directory: %s\n", target)
		os.Exit(1)
	}

	fsfd, err := fsopen(fstype, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsopen(%s) failed: %v\n", fstype, err)
		os.Exit(1)
	}
	defer unix.Close(fsfd)

	// if a source string was provided, set it as an fsconfig string 'source'
	if source != "" {
		if err := fsconfig(fsfd, FSCONFIG_SET_STRING, bs("source"), bs(source), 0); err != nil {
			fmt.Fprintf(os.Stderr, "fsconfig set source=%s failed: %v\n", source, err)
			os.Exit(1)
		}
	}

	// apply options
	for _, kv := range opts {
		parts := strings.SplitN(kv, "=", 2)
		key := parts[0]
		var val string
		if len(parts) > 1 {
			val = parts[1]
		}
		if val == "" {
			// set flag
			if err := fsconfig(fsfd, FSCONFIG_SET_FLAG, bs(key), nil, 0); err != nil {
				fmt.Fprintf(os.Stderr, "fsconfig set flag %s failed: %v\n", key, err)
				os.Exit(1)
			}
		} else {
			if err := fsconfig(fsfd, FSCONFIG_SET_STRING, bs(key), bs(val), 0); err != nil {
				fmt.Fprintf(os.Stderr, "fsconfig set %s=%s failed: %v\n", key, val, err)
				os.Exit(1)
			}
		}
	}

	// create the fs context
	if err := fsconfig(fsfd, FSCONFIG_CMD_CREATE, nil, nil, 0); err != nil {
		fmt.Fprintf(os.Stderr, "fsconfig create failed: %v\n", err)
		os.Exit(1)
	}

	mfd, err := fsmount(fsfd, 0, 0)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fsmount failed: %v\n", err)
		os.Exit(1)
	}
	defer unix.Close(mfd)

	// If a mount namespace was provided, open it and perform a move_mount into that namespace.
	if mountNS != "" {
		// Lock thread because setns affects the current thread
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		// Open target namespace
		nsFd, err := unix.Open(mountNS, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open mount namespace %s failed: %v\n", mountNS, err)
			os.Exit(1)
		}
		defer unix.Close(nsFd)

		// Create the mount path inside target namespace: we need to ensure it exists there.
		// To do that, open the namespace, setns into it, create the path, then switch back.
		// Save current namespace fd to restore later
		selfNs, err := unix.Open("/proc/self/ns/mnt", unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			fmt.Fprintf(os.Stderr, "open self ns failed: %v\n", err)
			os.Exit(1)
		}
		// setns into target namespace
		if err := unix.Setns(nsFd, unix.CLONE_NEWNS); err != nil {
			unix.Close(selfNs)
			fmt.Fprintf(os.Stderr, "setns to %s failed: %v\n", mountNS, err)
			os.Exit(1)
		}

		// Ensure target path exists in the target namespace
		if err := os.MkdirAll(target, 0755); err != nil {
			// try restore and exit
			_ = unix.Setns(selfNs, unix.CLONE_NEWNS)
			unix.Close(selfNs)
			fmt.Fprintf(os.Stderr, "mkdir in target ns failed: %v\n", err)
			os.Exit(1)
		}

		// restore original namespace
		if err := unix.Setns(selfNs, unix.CLONE_NEWNS); err != nil {
			unix.Close(selfNs)
			fmt.Fprintf(os.Stderr, "restore self ns failed: %v\n", err)
			os.Exit(1)
		}
		unix.Close(selfNs)

		// Now move the mount from this namespace into the target namespace by using move_mount
		// The to_dfd should be the file descriptor of the target namespace's mount namespace via open_tree?
		// Instead we use move_mount with to_dfd = AT_FDCWD while in target ns: perform setns into target and call move_mount.

		// setns into target namespace again to perform the attach there
		if err := unix.Setns(nsFd, unix.CLONE_NEWNS); err != nil {
			fmt.Fprintf(os.Stderr, "setns to %s for attach failed: %v\n", mountNS, err)
			os.Exit(1)
		}

		// perform move_mount: since we're in target ns, moving mfd (which refers to a mount in the original ns)
		// we need to call move_mount with from_dfd = mfd and to_dfd = AT_FDCWD, path = target
		// However syscalls expect file descriptors, so we use from_dfd = mfd and from_path empty.
		if err := moveMount(mfd, "", unix.AT_FDCWD, target, MOVE_MOUNT_F_EMPTY_PATH); err != nil {
			fmt.Fprintf(os.Stderr, "move_mount into target ns failed: %v\n", err)
			os.Exit(1)
		}

		// restore to original namespace
		// open self ns and restore
		// Note: we locked the OS thread so other goroutines won't be affected
		// The original ns was already restored above after creating the path, but ensure we restore to the current process's ns by opening /proc/self/ns/mnt and setns to it.
		// (no-op here)

		fmt.Printf("mounted %s at %s (fstype=%s) and moved into namespace %s\n", fstype, target, fstype, mountNS)
	} else {
		// attach the mount to the target path in current namespace
		// Use MOVE_MOUNT_F_EMPTY_PATH so that from_path can be empty which means use the mount itself
		if err := moveMount(mfd, "", unix.AT_FDCWD, target, MOVE_MOUNT_F_EMPTY_PATH); err != nil {
			fmt.Fprintf(os.Stderr, "move_mount attach failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("mounted %s at %s (fstype=%s)\n", fstype, target, fstype)
	}
}

// multiString allows repeated -o flags
type multiString []string

func (m *multiString) String() string { return strings.Join(*m, ",") }
func (m *multiString) Set(v string) error {
	*m = append(*m, v)
	return nil
}
