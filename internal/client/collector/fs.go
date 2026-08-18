package collector

import (
	"syscall"

	"agentboard/internal/event"
)

// readFilesystems returns filesystem usage for the configured mounts using
// statfs, plus the root ("/") used/total for the metric sample top-level fields.
func readFilesystems(includeMounts, _ []string) (fss []event.Filesystem, rootUsed, rootTotal *int64) {
	mounts := includeMounts
	if len(mounts) == 0 {
		mounts = []string{"/"}
	}
	for _, m := range mounts {
		var st syscall.Statfs_t
		if err := syscall.Statfs(m, &st); err != nil {
			continue
		}
		bsize := int64(st.Bsize)
		total := int64(st.Blocks) * bsize
		free := int64(st.Bavail) * bsize
		used := total - free
		fss = append(fss, event.Filesystem{Mount: m, UsedBytes: used, TotalBytes: total})
		if m == "/" {
			u, t := used, total
			rootUsed, rootTotal = &u, &t
		}
	}
	if rootUsed == nil && len(fss) > 0 {
		u, t := fss[0].UsedBytes, fss[0].TotalBytes
		rootUsed, rootTotal = &u, &t
	}
	return fss, rootUsed, rootTotal
}
