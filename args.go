package goclikit

import "os"

// commandLineArgs is os.Args without the program name.
func commandLineArgs() []string {
	if len(os.Args) < 2 {
		return nil
	}
	return os.Args[1:]
}
