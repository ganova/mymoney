//go:build !windows

package ai

import "os/exec"

func hideWindow(*exec.Cmd) {}
