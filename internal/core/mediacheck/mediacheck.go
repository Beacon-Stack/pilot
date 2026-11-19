// Package mediacheck rejects video files that hold no data.
//
// An incomplete download can leave a file with the right name and size
// whose bytes are all zero: the client preallocated it and never wrote
// the pieces. Recorded as an episode file, it marks the episode as "have
// file" forever, so nothing re-downloads it. Import and the library scan
// call Check before recording a file.
package mediacheck

import (
	"errors"
	"fmt"
	"io"
	"os"
)

// ErrNoData is returned for a file that is empty or whose sampled bytes
// are all zero.
var ErrNoData = errors.New("file contains no data (empty, or all zeros: an incomplete download?)")

const (
	sampleSize = 64 << 10 // bytes read at each sample point
	samples    = 8        // sample points spread across the file
)

// Check reads a few small chunks spread across path and returns ErrNoData
// when every one of them is zero (or the file is empty). Any real video
// container has non-zero bytes in its header, so a healthy file passes on
// the first read. A partially downloaded file with some pieces written
// passes too: Check only catches files with no data at all.
func Check(path string) error {
	f, err := os.Open(path) //nolint:gosec // caller-provided library/download path
	if err != nil {
		return err
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return err
	}
	size := fi.Size()
	if size == 0 {
		return fmt.Errorf("%s: %w", path, ErrNoData)
	}

	buf := make([]byte, sampleSize)
	for i := int64(0); i <= samples; i++ {
		off := size * i / samples // the last sample (i == samples) is the tail
		if i == samples {
			off = max(size-sampleSize, 0)
		}
		n, err := f.ReadAt(buf, off)
		if err != nil && !errors.Is(err, io.EOF) {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		for _, b := range buf[:n] {
			if b != 0 {
				return nil
			}
		}
	}
	return fmt.Errorf("%s: %w", path, ErrNoData)
}
