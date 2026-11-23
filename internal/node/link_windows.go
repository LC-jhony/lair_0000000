//go:build windows

package node

import (
	"fmt"
	"os"
	"os/exec"
)

// linkCurrent points the stable `current` path at the npm prefix of the
// default Node major. Windows uses a directory junction: unlike a symlink it
// needs no developer mode and no elevated privileges.
func linkCurrent(target, link string) error {
	if info, err := os.Lstat(link); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("refusing to replace %s: not a directory junction", link)
		}
		if err := os.Remove(link); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	out, err := exec.Command("cmd", "/c", "mklink", "/J", link, target).CombinedOutput()
	if err != nil {
		return fmt.Errorf("mklink /J %s: %w (%s)", link, err, string(out))
	}
	return nil
}
