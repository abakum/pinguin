//go:build windows

package main

import (
	"fmt"
	"unicode/utf16"

	"github.com/danieljoos/wincred"
)

func credLookup(key string) string {
	cred, err := wincred.GetGenericCredential(key)
	if err != nil {
		ltf.Println("wincred.GetGenericCredential", key, err)
		// fallback: scan the credential list for the target name
		list, lerr := wincred.List()
		if lerr != nil {
			ltf.Println("wincred.List", lerr)
			return ""
		}
		for _, c := range list {
			if c.TargetName != key {
				continue
			}
			ltf.Println("wincred: found in list", key, "blob", len(c.CredentialBlob), "bytes")
			return decodeBlob(c.CredentialBlob)
		}
		ltf.Println("wincred: not found in list", key)
		return ""
	}
	v := decodeBlob(cred.CredentialBlob)
	if key == "pinguin_token" {
		ltf.Println("wincred", key, len(cred.CredentialBlob), "bytes, decoded", len(v), "chars")
	} else {
		ltf.Println("wincred", key, len(cred.CredentialBlob), fmt.Sprintf("%q", v))
	}
	return v
}

// cmdkey stores the secret as UTF-16LE, credentials created by other tools may be plain bytes
func decodeBlob(b []byte) string {
	if len(b) > 1 && len(b)%2 == 0 {
		le := true
		for i := 1; i < len(b); i += 2 {
			if b[i] != 0 {
				le = false
				break
			}
		}
		if le {
			u := make([]uint16, len(b)/2)
			for i := range u {
				u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
			}
			return string(utf16.Decode(u))
		}
	}
	return string(b)
}
