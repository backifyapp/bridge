//go:build !linux && !darwin

package dockerhttp

// diskFree is unknown on other platforms (the Bridge ships for Linux only);
// the import then relies on the Content-Length requirement alone.
func diskFree(string) (uint64, bool) { return 0, false }
