//go:build windows

package runner

// Windows has no sessions to escape into: taskkill /T follows the process tree.
func startEscapee() int { return 99 }
