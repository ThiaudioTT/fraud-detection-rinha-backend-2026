package index

import (
	"fmt"
	"os"
	"syscall"
)

// Open mmap's references.bin read-only and returns a searchable Index. The
// mapping is MAP_PRIVATE+PROT_READ so the pages stay clean and shareable across
// processes that map the same underlying file (e.g. both API containers reading
// the same image layer). The data lives off the Go heap, so the GC never scans
// it.
func Open(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("index: open %s: %w", path, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("index: stat %s: %w", path, err)
	}
	size := int(fi.Size())
	if size < headerSize {
		return nil, fmt.Errorf("index: %s too small (%d bytes)", path, size)
	}

	data, err := syscall.Mmap(int(f.Fd()), 0, size, syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		return nil, fmt.Errorf("index: mmap %s: %w", path, err)
	}

	// Advise the kernel we will touch these pages randomly and want them
	// resident; failure is non-fatal (best-effort warm-up).
	_ = syscall.Madvise(data, syscall.MADV_WILLNEED)

	ix, err := fromMmap(data)
	if err != nil {
		_ = syscall.Munmap(data)
		return nil, err
	}
	return ix, nil
}

// Close releases the mmap. After Close the Index must not be used.
func (ix *Index) Close() error {
	if ix.mmapData == nil {
		return nil
	}
	data := ix.mmapData
	ix.mmapData = nil
	ix.centroids = nil
	ix.offsets = nil
	ix.vectors = nil
	ix.labels = nil
	return syscall.Munmap(data)
}
