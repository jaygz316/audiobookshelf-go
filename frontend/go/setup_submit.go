//go:build js && wasm

package main

import (
	"fmt"
	"strings"
	"syscall/js"
)

func submitSetupForm(doc js.Value, errEl js.Value, successEl js.Value, submitBtn js.Value) {
	errEl.Get("classList").Call("add", "hidden")
	successEl.Get("classList").Call("add", "hidden")

	submitBtn.Set("disabled", true)
	submitBtn.Set("textContent", "Initializing...")

	username := strings.TrimSpace(doc.Call("getElementById", "setup-username").Get("value").String())
	if username == "" {
		username = "root"
	}
	password := doc.Call("getElementById", "setup-password").Get("value").String()

	initPayload := map[string]any{
		"newRoot": map[string]string{
			"username": username,
			"password": password,
		},
	}

	// 1. Create root user
	_, err := apiRequest("POST", "/init", initPayload)
	if err != nil {
		errEl.Set("textContent", "Server init failed: "+err.Error())
		errEl.Get("classList").Call("remove", "hidden")
		submitBtn.Set("disabled", false)
		submitBtn.Set("textContent", "Finish")
		return
	}

	successEl.Set("textContent", "Account created! Signing you in...")
	successEl.Get("classList").Call("remove", "hidden")

	// 2. Login to get token
	loginPayload, err := apiRequest("POST", "/login", map[string]string{
		"username": username,
		"password": password,
	})
	if err != nil {
		errEl.Set("textContent", "Login failed: "+err.Error())
		errEl.Get("classList").Call("remove", "hidden")
		submitBtn.Set("disabled", false)
		submitBtn.Set("textContent", "Finish")
		return
	}

	var tokenStr string
	if loginPayload.Type() != js.TypeUndefined && loginPayload.Type() != js.TypeNull {
		user := loginPayload.Get("user")
		if user.Type() != js.TypeUndefined && user.Type() != js.TypeNull {
			tok := user.Get("accessToken")
			if tok.Type() == js.TypeUndefined || tok.Type() == js.TypeNull {
				tok = user.Get("token")
			}
			if tok.Type() != js.TypeUndefined && tok.String() != "" {
				tokenStr = tok.String()
				js.Global().Get("localStorage").Call("setItem", "token", tok)
			}
		}
	}

	if tokenStr == "" {
		errEl.Set("textContent", "No access token returned from login.")
		errEl.Get("classList").Call("remove", "hidden")
		submitBtn.Set("disabled", false)
		submitBtn.Set("textContent", "Finish")
		return
	}

	// 3. Create configured library
	libName := strings.TrimSpace(doc.Call("getElementById", "setup-library-name").Get("value").String())
	libPath := strings.TrimSpace(doc.Call("getElementById", "setup-library-path").Get("value").String())
	libType := "book"
	radioChecked := doc.Call("querySelector", "input[name='setup-library-type']:checked")
	if radioChecked.Type() != js.TypeNull {
		libType = radioChecked.Get("value").String()
	}

	iconName := "audiobooks"
	if libType == "podcast" {
		iconName = "podcasts"
	}

	libraryPayload := map[string]any{
		"name":      libName,
		"mediaType": libType,
		"icon":      iconName,
		"folders": []map[string]string{
			{"path": libPath},
		},
	}

	_, err = apiRequest("POST", "/api/libraries", libraryPayload)
	if err != nil {
		fmt.Printf("Failed to create initial library: %v\n", err)
	}

	successEl.Set("textContent", "Setup complete!")

	doc.Call("getElementById", "setup-screen").Get("classList").Call("add", "hidden")
	showAppContainerGo(js.Undefined(), nil)

	// Dispatch abs:authed event
	customEvent := js.Global().Get("CustomEvent").New("abs:authed", map[string]any{
		"detail": loginPayload,
	})
	js.Global().Call("dispatchEvent", customEvent)
}
