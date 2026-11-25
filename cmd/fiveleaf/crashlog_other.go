//go:build !darwin

package main

// keepSystemCrashes keeps the system's reports of FiveLeaf's crashes, which
// only macOS writes.
func keepSystemCrashes(dir string) {}
