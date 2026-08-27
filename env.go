package main

import (
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// optional per-platform lookup, implemented in creds_windows.go / creds_other.go
var loadOnce sync.Once

// lookup order: environment -> .env (godotenv, does not override) -> Windows Credential Manager
func envValue(key string) string {
	loadOnce.Do(func() {
		_ = godotenv.Load()
	})
	if v := os.Getenv(key); v != "" {
		return v
	}
	if v := credLookup(key); v != "" {
		return v
	}
	return ""
}

// split pinguin_ids by spaces and/or commas, drop non-numeric items
func parseIDs(s string) []int {
	var ids []int
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		if i, err := strconv.Atoi(f); err == nil {
			ids = append(ids, i)
		}
	}
	return ids
}
