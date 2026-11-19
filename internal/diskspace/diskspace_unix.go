//go:build !windows

package diskspace

import "golang.org/x/sys/unix"

func free(path string) (uint64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil // Bsize is int64 on Linux, uint32 on macOS
}
