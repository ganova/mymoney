package ocr

import (
	"os/exec"
	"syscall"
)

// hideWindow mencegah jendela konsol muncul saat memanggil tesseract di Windows.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
