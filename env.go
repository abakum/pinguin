package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/joho/godotenv"
)

// directory of the executable; cwd on failure (elevated relaunch may get System32 cwd)
func exeDir() string {
	if ex, err := os.Executable(); err == nil {
		return filepath.Dir(ex)
	}
	if ex, err := os.Getwd(); err == nil {
		return ex
	}
	return "."
}

// optional per-platform lookup, implemented in creds_windows.go / creds_other.go
var loadOnce sync.Once

// lookup order: environment -> .env (godotenv, does not override) -> Windows Credential Manager
func envValue(key string) string {
	loadOnce.Do(func() {
		_ = godotenv.Load(filepath.Join(exeDir(), ".env"))
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
