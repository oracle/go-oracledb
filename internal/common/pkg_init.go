package common

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

var (
	// ProgramName is the executable base name captured during package initialization.
	ProgramName string
	// HostName is the local hostname captured during package initialization.
	HostName string
	// UserName is the operating-system username captured during package initialization.
	UserName string
	// ProcessID is the current process ID captured during package initialization.
	ProcessID string
)

// initSystemInformation captures process metadata once in the common layer so
// driver components avoid repeating operating-system calls.
func initSystemInformation() {
	ProgramName = "unknown"
	if executable, err := os.Executable(); err == nil {
		if name := filepath.Base(executable); name != "" {
			ProgramName = name
		}
	}

	HostName = "unknown"
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		HostName = hostname
	}

	UserName = "oraclegoclient"
	if currentUser, err := user.Current(); err == nil && currentUser.Username != "" {
		UserName = currentUser.Username
	}

	ProcessID = strconv.Itoa(os.Getpid())
}

// init initializes shared system information and error-message dictionaries.
func init() {
	initSystemInformation()
	initMessagesEn()
	initMessagesFr()
}
