//go:build !windows

package ocr

import "os/exec"

func hideWindow(*exec.Cmd) {}
