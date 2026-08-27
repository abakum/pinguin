//go:build !windows

package main

func credLookup(string) string {
	return ""
}
