//go:build !windows

package node

import (
	"fmt"
	"os"
)

// linkCurrent points the stable `current` path at the npm prefix of the
// default Node major. On Unix it is a symlink; a real directory is never
// replaced, so an operator's data cannot be destroyed by a bad config.
func linkCurrent(target, link string) error {
	if info, err := os.Lstat(link); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return fmt.Errorf("refusing to replace %s: not a symlink", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Symlink(target, link)
}
