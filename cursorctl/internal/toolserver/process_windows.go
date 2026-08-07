//go:build windows

package toolserver

import "os/exec"

func configureProcessCancellation(_ *exec.Cmd) {}
