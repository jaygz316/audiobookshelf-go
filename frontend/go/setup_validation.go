//go:build js && wasm

package main

import (
	"strings"
	"syscall/js"
)

func validateStep1(doc js.Value, errEl js.Value) bool {
	password := doc.Call("getElementById", "setup-password").Get("value").String()
	password2 := doc.Call("getElementById", "setup-password2").Get("value").String()

	errEl.Get("classList").Call("add", "hidden")

	if password != password2 {
		errEl.Set("textContent", "Passwords do not match.")
		errEl.Get("classList").Call("remove", "hidden")
		return false
	}
	if len(password) < 5 {
		errEl.Set("textContent", "Password must be at least 5 characters.")
		errEl.Get("classList").Call("remove", "hidden")
		return false
	}
	return true
}

func validateStep2(doc js.Value, errEl js.Value) bool {
	libName := strings.TrimSpace(doc.Call("getElementById", "setup-library-name").Get("value").String())
	libPath := strings.TrimSpace(doc.Call("getElementById", "setup-library-path").Get("value").String())

	errEl.Get("classList").Call("add", "hidden")

	if libName == "" {
		errEl.Set("textContent", "Library Name is required.")
		errEl.Get("classList").Call("remove", "hidden")
		return false
	}
	if libPath == "" {
		errEl.Set("textContent", "Folder Path is required.")
		errEl.Get("classList").Call("remove", "hidden")
		return false
	}
	return true
}
