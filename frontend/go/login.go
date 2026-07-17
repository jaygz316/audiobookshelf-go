//go:build js && wasm

package main

import (
	"fmt"
	"syscall/js"
)

func showLoginScreenGo(this js.Value, args []js.Value) any {
	doc := js.Global().Get("document")
	doc.Call("getElementById", "login-screen").Get("classList").Call("remove", "hidden")
	doc.Call("getElementById", "setup-screen").Get("classList").Call("add", "hidden")
	doc.Call("getElementById", "app-container").Get("classList").Call("add", "hidden")

	authWarning := doc.Call("getElementById", "login-auth-warning")
	if authWarning.Type() != js.TypeNull {
		authWarning.Get("classList").Call("add", "hidden")
		authWarning.Get("classList").Call("remove", "flex")
	}

	var status js.Value
	if len(args) > 0 {
		status = args[0]
	}

	applyStatusToLoginScreenGo(status)
	return nil
}

func applyStatusToLoginScreenGo(status js.Value) {
	doc := js.Global().Get("document")
	localForm := doc.Call("getElementById", "login-form")
	oidcBtn := doc.Call("getElementById", "oidc-btn")
	divider := doc.Call("getElementById", "login-divider")
	customMessageEl := doc.Call("getElementById", "login-custom-message")

	var methods []string
	if status.Type() != js.TypeUndefined && status.Type() != js.TypeNull && status.Get("authMethods").Type() != js.TypeUndefined {
		length := status.Get("authMethods").Get("length").Int()
		for i := 0; i < length; i++ {
			methods = append(methods, status.Get("authMethods").Index(i).String())
		}
	} else {
		methods = []string{"local"}
	}

	hasLocal := false
	hasOpenID := false
	for _, m := range methods {
		if m == "local" {
			hasLocal = true
		}
		if m == "openid" {
			hasOpenID = true
		}
	}

	if hasLocal && localForm.Type() != js.TypeNull {
		localForm.Get("classList").Call("remove", "hidden")
	} else if localForm.Type() != js.TypeNull {
		localForm.Get("classList").Call("add", "hidden")
	}

	if customMessageEl.Type() != js.TypeNull {
		hasCustom := false
		var customHTML string
		if status.Type() != js.TypeUndefined && status.Type() != js.TypeNull {
			formData := status.Get("authFormData")
			if formData.Type() != js.TypeUndefined && formData.Type() != js.TypeNull {
				msgVal := formData.Get("authLoginCustomMessage")
				if msgVal.Type() != js.TypeUndefined && msgVal.Type() != js.TypeNull && msgVal.String() != "" {
					hasCustom = true
					customHTML = msgVal.String()
				}
			}
		}

		if hasCustom {
			sanitized := sanitizeHTMLGo(js.Undefined(), []js.Value{js.ValueOf(customHTML)}).(string)
			customMessageEl.Set("innerHTML", sanitized)
			customMessageEl.Get("classList").Call("remove", "hidden")
		} else {
			customMessageEl.Get("classList").Call("add", "hidden")
			customMessageEl.Set("innerHTML", "")
		}
	}

	if hasOpenID && oidcBtn.Type() != js.TypeNull {
		oidcBtn.Get("classList").Call("remove", "hidden")
		if divider.Type() != js.TypeNull {
			divider.Get("classList").Call("remove", "hidden")
		}

		oidcBtnText := doc.Call("getElementById", "oidc-btn-text")
		btnText := "Sign in with OpenId"
		autoLaunch := false

		if status.Type() != js.TypeUndefined && status.Type() != js.TypeNull {
			formData := status.Get("authFormData")
			if formData.Type() != js.TypeUndefined && formData.Type() != js.TypeNull {
				textVal := formData.Get("authOpenIDButtonText")
				if textVal.Type() != js.TypeUndefined && textVal.Type() != js.TypeNull && textVal.String() != "" {
					btnText = textVal.String()
				}
				autoLaunchVal := formData.Get("authOpenIDAutoLaunch")
				if autoLaunchVal.Type() != js.TypeUndefined && autoLaunchVal.Type() != js.TypeNull {
					autoLaunch = autoLaunchVal.Bool()
				}
			}
		}

		if oidcBtnText.Type() != js.TypeNull {
			oidcBtnText.Set("textContent", btnText)
		}

		triggerOidcRedirect := func() {
			window := js.Global().Get("window")
			origin := window.Get("location").Get("origin").String()
			pathname := window.Get("location").Get("pathname").String()
			callbackURL := js.Global().Call("encodeURIComponent", origin+pathname).String()

			resolvedPath := js.Global().Call("resolvePath", fmt.Sprintf("/auth/openid?redirect=%s", callbackURL)).String()
			window.Get("location").Set("href", resolvedPath)
		}

		oidcBtn.Set("onclick", js.FuncOf(func(oidcThis js.Value, oidcArgs []js.Value) any {
			triggerOidcRedirect()
			return nil
		}))

		if autoLaunch {
			urlParams := js.Global().Get("URLSearchParams").New(js.Global().Get("window").Get("location").Get("search"))
			if !urlParams.Call("has", "local").Bool() && !urlParams.Call("has", "bypass").Bool() {
				triggerOidcRedirect()
			}
		}
	} else if oidcBtn.Type() != js.TypeNull {
		oidcBtn.Get("classList").Call("add", "hidden")
		if divider.Type() != js.TypeNull {
			divider.Get("classList").Call("add", "hidden")
		}
	}
}

func showAppContainerGo(this js.Value, args []js.Value) any {
	doc := js.Global().Get("document")
	doc.Call("getElementById", "login-screen").Get("classList").Call("add", "hidden")
	doc.Call("getElementById", "setup-screen").Get("classList").Call("add", "hidden")
	doc.Call("getElementById", "app-container").Get("classList").Call("remove", "hidden")
	return nil
}
