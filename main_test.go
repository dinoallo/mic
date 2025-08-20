package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestParseArgs_MissingTarget(t *testing.T) {
	_, _, _, _, _, err := parseArgs([]string{"-fstype", "tmpfs"})
	if err == nil {
		t.Fatalf("expected error for missing -target, got nil")
	}
}

// TestMountOperation is an integration-style test that actually performs a mount.
// It is skipped by default; to run it set RUN_MOUNT_TESTS=1 and run as root.
func TestMountOperation(t *testing.T) {
	if os.Getenv("RUN_MOUNT_TESTS") != "1" {
		t.Skip("skipping mount integration test; set RUN_MOUNT_TESTS=1 to run")
	}
	if os.Geteuid() != 0 {
		t.Skip("mount tests require root; run as root or with appropriate capabilities")
	}

	tmpdir, err := os.MkdirTemp("", "mic-mounttest-")
	if err != nil {
		t.Fatalf("mkdir temp: %v", err)
	}
	defer os.RemoveAll(tmpdir)

	// Build binary to a temp location
	bin := filepath.Join(tmpdir, "micbin")
	build := exec.Command("go", "build", "-o", bin, "./")
	build.Env = os.Environ()
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v: %s", err, string(out))
	}

	// Run the binary to mount tmpfs at tmpdir
	cmd := exec.Command(bin, "-target", tmpdir, "-fstype", "tmpfs", "-o", "size=4M")
	cmd.Env = os.Environ()
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mount binary failed: %v: %s", err, string(out))
	}

	// small delay to ensure mount is visible
	time.Sleep(100 * time.Millisecond)

	mounts, err := os.ReadFile("/proc/mounts")
	if err != nil {
		// attempt cleanup
		_ = unix.Unmount(tmpdir, 0)
		t.Fatalf("read /proc/mounts failed: %v", err)
	}
	if !strings.Contains(string(mounts), tmpdir) {
		_ = unix.Unmount(tmpdir, 0)
		t.Fatalf("mount not found in /proc/mounts")
	}

	// Unmount and cleanup
	if err := unix.Unmount(tmpdir, 0); err != nil {
		t.Fatalf("unmount failed: %v", err)
	}
}

func TestParseArgs_DefaultFstypeAndSingleOption(t *testing.T) {
	target, fstype, mountNS, source, opts, err := parseArgs([]string{"-target", "/tmp", "-o", "size=64M"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if target != "/tmp" {
		t.Fatalf("expected target /tmp, got %s", target)
	}
	if fstype != "tmpfs" {
		t.Fatalf("expected default fstype tmpfs, got %s", fstype)
	}
	if mountNS != "" {
		t.Fatalf("expected empty mountNS by default, got %s", mountNS)
	}
	if source != "" {
		t.Fatalf("expected empty source by default, got %s", source)
	}
	if !reflect.DeepEqual(opts, []string{"size=64M"}) {
		t.Fatalf("expected opts [size=64M], got %#v", opts)
	}
}

func TestParseArgs_MultipleOptionsAndFstype(t *testing.T) {
	target, fstype, mountNS, source, opts, err := parseArgs([]string{"-target", "/mnt/t", "-fstype", "fuse.blah", "-o", "a=1", "-o", "flagonly"})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if target != "/mnt/t" {
		t.Fatalf("expected target /mnt/t, got %s", target)
	}
	if fstype != "fuse.blah" {
		t.Fatalf("expected fstype fuse.blah, got %s", fstype)
	}
	if mountNS != "" {
		t.Fatalf("expected empty mountNS by default, got %s", mountNS)
	}
	if source != "" {
		t.Fatalf("expected empty source by default, got %s", source)
	}
	if !reflect.DeepEqual(opts, []string{"a=1", "flagonly"}) {
		t.Fatalf("expected opts [a=1 flagonly], got %#v", opts)
	}
}
