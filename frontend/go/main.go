package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"syscall/js"
)

func main() {
	// Expose Go functions to JS scope
	js.Global().Set("initAuthGo", js.FuncOf(initAuthGo))
	js.Global().Set("logoutGo", js.FuncOf(logoutGo))
	js.Global().Set("showSetupScreenGo", js.FuncOf(showSetupScreenGo))
	js.Global().Set("showLoginScreenGo", js.FuncOf(showLoginScreenGo))
	js.Global().Set("showAppContainerGo", js.FuncOf(showAppContainerGo))
	js.Global().Set("sanitizeHTMLGo", js.FuncOf(sanitizeHTMLGo))

	// Keep the WebAssembly program running in background
	select {}
}

// helper to convert any Go value to a JS value using JSON roundtrip
func valueToJS(v any) js.Value {
	data, err := json.Marshal(v)
	if err != nil {
		return js.Null()
	}
	return js.Global().Get("JSON").Call("parse", string(data))
}

// helper to await a JS Promise using Go channels
func awaitPromise(promise js.Value) (js.Value, error) {
	ch := make(chan js.Value, 1)
	errCh := make(chan error, 1)

	thenFunc := js.FuncOf(func(this js.Value, args []js.Value) any {
		var val js.Value
		if len(args) > 0 {
			val = args[0]
		} else {
			val = js.Undefined()
		}
		ch <- val
		return nil
	})
	defer thenFunc.Release()

	catchFunc := js.FuncOf(func(this js.Value, args []js.Value) any {
		var errMsg string
		if len(args) > 0 {
			if args[0].Get("message").Type() != js.TypeUndefined {
				errMsg = args[0].Get("message").String()
			} else {
				errMsg = args[0].String()
			}
		} else {
			errMsg = "unknown promise error"
		}
		errCh <- fmt.Errorf("%s", errMsg)
		return nil
	})
	defer catchFunc.Release()

	promise.Call("then", thenFunc).Call("catch", catchFunc)

	select {
	case val := <-ch:
		return val, nil
	case err := <-errCh:
		return js.Undefined(), err
	}
}

// helper to trigger HTTP API requests via JS's global apiRequest wrapper
func apiRequest(method, path string, body any) (js.Value, error) {
	jsMethod := js.ValueOf(method)
	jsPath := js.ValueOf(path)
	var jsBody js.Value
	if body != nil {
		jsBody = valueToJS(body)
	} else {
		jsBody = js.Null()
	}

	promise := js.Global().Call("apiRequest", jsMethod, jsPath, jsBody)
	return awaitPromise(promise)
}

func initAuthGo(this js.Value, args []js.Value) any {
	return js.Global().Get("Promise").New(js.FuncOf(func(promiseThis js.Value, promiseArgs []js.Value) any {
		resolve := promiseArgs[0]
		_ = promiseArgs[1]

		go func() {
			window := js.Global().Get("window")
			loc := window.Get("location")
			urlParams := js.Global().Get("URLSearchParams").New(loc.Get("search"))

			tokenVal := urlParams.Call("get", "accessToken")
			if tokenVal.Type() == js.TypeUndefined || tokenVal.Type() == js.TypeNull {
				tokenVal = urlParams.Call("get", "setToken")
			}

			if tokenVal.Type() != js.TypeUndefined && tokenVal.Type() != js.TypeNull && tokenVal.String() != "" {
				js.Global().Get("localStorage").Call("setItem", "token", tokenVal)
				urlParams.Call("delete", "accessToken")
				urlParams.Call("delete", "setToken")
				urlParams.Call("delete", "state")

				cleanURL := loc.Get("pathname").String()
				paramsStr := urlParams.Call("toString").String()
				if paramsStr != "" {
					cleanURL += "?" + paramsStr
				}
				window.Get("history").Call("replaceState", js.ValueOf(map[string]any{}), js.Global().Get("document").Get("title"), cleanURL)
			}

			status, err := apiRequest("GET", "/status", nil)
			if err != nil {
				fmt.Printf("Failed to fetch /status: %v\n", err)
			} else {
				js.Global().Set("serverStatus", status)
			}

			isInit := true
			if status.Type() != js.TypeUndefined && status.Type() != js.TypeNull {
				if status.Get("isInit").Type() != js.TypeUndefined {
					isInit = status.Get("isInit").Bool()
				}
			}

			if !isInit {
				showSetupScreenGo(js.Undefined(), []js.Value{status})
				resolve.Invoke(nil)
				return
			}

			token := js.Global().Get("localStorage").Call("getItem", "token")
			if token.Type() == js.TypeUndefined || token.Type() == js.TypeNull || token.String() == "" {
				showLoginScreenGo(js.Undefined(), []js.Value{status})
				resolve.Invoke(nil)
				return
			}

			payload, err := apiRequest("POST", "/api/authorize", nil)
			if err != nil {
				fmt.Printf("Initial authorize failed: %v\n", err)
				js.Global().Get("localStorage").Call("removeItem", "token")
				showLoginScreenGo(js.Undefined(), []js.Value{status})
				resolve.Invoke(nil)
				return
			}

			if payload.Type() != js.TypeUndefined && payload.Type() != js.TypeNull {
				user := payload.Get("user")
				if user.Type() != js.TypeUndefined && user.Type() != js.TypeNull {
					if user.Get("isOldToken").Type() != js.TypeUndefined && user.Get("isOldToken").Bool() {
						fmt.Println("User has old token. Forcing re-login.")
						js.Global().Get("localStorage").Call("removeItem", "token")
						showLoginScreenGo(js.Undefined(), []js.Value{status})

						usernameInput := js.Global().Get("document").Call("getElementById", "username")
						if usernameInput.Type() != js.TypeNull {
							usernameInput.Set("value", user.Get("username"))
						}

						authWarning := js.Global().Get("document").Call("getElementById", "login-auth-warning")
						if authWarning.Type() != js.TypeNull {
							authWarning.Get("classList").Call("remove", "hidden")
							authWarning.Get("classList").Call("add", "flex")

							moreInfoLink := js.Global().Get("document").Call("getElementById", "login-auth-warning-more-info")
							if moreInfoLink.Type() != js.TypeNull {
								userType := user.Get("type").String()
								if userType == "admin" || userType == "root" {
									moreInfoLink.Get("classList").Call("remove", "hidden")
								} else {
									moreInfoLink.Get("classList").Call("add", "hidden")
								}
							}
						}
						resolve.Invoke(nil)
						return
					}
				}
			}

			showAppContainerGo(js.Undefined(), nil)

			// Convert payload object back to JS context return
			var result any
			json.Unmarshal([]byte(js.Global().Get("JSON").Call("stringify", payload).String()), &result)
			resolve.Invoke(valueToJS(result))
		}()
		return nil
	}))
}

func showSetupScreenGo(this js.Value, args []js.Value) any {
	doc := js.Global().Get("document")
	doc.Call("getElementById", "setup-screen").Get("classList").Call("remove", "hidden")
	doc.Call("getElementById", "login-screen").Get("classList").Call("add", "hidden")
	doc.Call("getElementById", "app-container").Get("classList").Call("add", "hidden")

	var status js.Value
	if len(args) > 0 {
		status = args[0]
	}

	if status.Type() != js.TypeUndefined && status.Type() != js.TypeNull {
		configPathEl := doc.Call("getElementById", "setup-config-path")
		metadataPathEl := doc.Call("getElementById", "setup-metadata-path")
		if configPathEl.Type() != js.TypeNull {
			configPathEl.Set("value", status.Get("ConfigPath"))
		}
		if metadataPathEl.Type() != js.TypeNull {
			metadataPathEl.Set("value", status.Get("MetadataPath"))
		}
	}

	currentStep := 1

	// Elements
	setupStep1 := doc.Call("getElementById", "setup-step-1")
	setupStep2 := doc.Call("getElementById", "setup-step-2")
	setupStep3 := doc.Call("getElementById", "setup-step-3")

	backBtn := doc.Call("getElementById", "setup-back-btn")
	nextBtn := doc.Call("getElementById", "setup-next-btn")
	submitBtn := doc.Call("getElementById", "setup-submit-btn")

	indicator1 := doc.Call("getElementById", "step-indicator-1")
	indicator2 := doc.Call("getElementById", "step-indicator-2")
	indicator3 := doc.Call("getElementById", "step-indicator-3")

	label1 := doc.Call("getElementById", "step-label-1")
	label2 := doc.Call("getElementById", "step-label-2")
	label3 := doc.Call("getElementById", "step-label-3")

	errEl := doc.Call("getElementById", "setup-error")
	successEl := doc.Call("getElementById", "setup-success")

	updateUI := func() {
		// Hide all steps
		setupStep1.Get("classList").Call("add", "hidden")
		setupStep2.Get("classList").Call("add", "hidden")
		setupStep3.Get("classList").Call("add", "hidden")

		// Reset indicators
		indicator1.Set("className", "w-8 h-8 rounded-full bg-black-600 text-gray-400 flex items-center justify-center font-bold text-sm transition-colors duration-150")
		indicator2.Set("className", "w-8 h-8 rounded-full bg-black-600 text-gray-400 flex items-center justify-center font-bold text-sm transition-colors duration-150")
		indicator3.Set("className", "w-8 h-8 rounded-full bg-black-600 text-gray-400 flex items-center justify-center font-bold text-sm transition-colors duration-150")

		label1.Set("className", "text-xs text-gray-400 mt-1 font-semibold")
		label2.Set("className", "text-xs text-gray-400 mt-1 font-semibold")
		label3.Set("className", "text-xs text-gray-400 mt-1 font-semibold")

		// Apply current step class values
		if currentStep == 1 {
			setupStep1.Get("classList").Call("remove", "hidden")
			indicator1.Set("className", "w-8 h-8 rounded-full bg-accent text-primary flex items-center justify-center font-bold text-sm transition-colors duration-150")
			label1.Set("className", "text-xs text-white mt-1 font-semibold")
			backBtn.Get("classList").Call("add", "hidden")
			nextBtn.Get("classList").Call("remove", "hidden")
			submitBtn.Get("classList").Call("add", "hidden")
		} else if currentStep == 2 {
			setupStep2.Get("classList").Call("remove", "hidden")
			indicator1.Set("className", "w-8 h-8 rounded-full bg-green-600 text-white flex items-center justify-center font-bold text-sm transition-colors duration-150")
			indicator2.Set("className", "w-8 h-8 rounded-full bg-accent text-primary flex items-center justify-center font-bold text-sm transition-colors duration-150")
			label2.Set("className", "text-xs text-white mt-1 font-semibold")
			backBtn.Get("classList").Call("remove", "hidden")
			nextBtn.Get("classList").Call("remove", "hidden")
			submitBtn.Get("classList").Call("add", "hidden")
		} else if currentStep == 3 {
			setupStep3.Get("classList").Call("remove", "hidden")
			indicator1.Set("className", "w-8 h-8 rounded-full bg-green-600 text-white flex items-center justify-center font-bold text-sm transition-colors duration-150")
			indicator2.Set("className", "w-8 h-8 rounded-full bg-green-600 text-white flex items-center justify-center font-bold text-sm transition-colors duration-150")
			indicator3.Set("className", "w-8 h-8 rounded-full bg-accent text-primary flex items-center justify-center font-bold text-sm transition-colors duration-150")
			label3.Set("className", "text-xs text-white mt-1 font-semibold")
			backBtn.Get("classList").Call("remove", "hidden")
			nextBtn.Get("classList").Call("add", "hidden")
			submitBtn.Get("classList").Call("remove", "hidden")

			// Populate summary text
			doc.Call("getElementById", "setup-summary-config").Set("textContent", doc.Call("getElementById", "setup-config-path").Get("value").String())
			doc.Call("getElementById", "setup-summary-metadata").Set("textContent", doc.Call("getElementById", "setup-metadata-path").Get("value").String())

			username := strings.TrimSpace(doc.Call("getElementById", "setup-username").Get("value").String())
			if username == "" {
				username = "root"
			}
			doc.Call("getElementById", "setup-summary-username").Set("textContent", username)
			doc.Call("getElementById", "setup-summary-libname").Set("textContent", doc.Call("getElementById", "setup-library-name").Get("value").String())

			// Radio check value
			radioChecked := doc.Call("querySelector", "input[name='setup-library-type']:checked")
			if radioChecked.Type() != js.TypeNull {
				doc.Call("getElementById", "setup-summary-libtype").Set("textContent", radioChecked.Get("value").String())
			}
		}
	}

	validateStep1 := func() bool {
		username := strings.TrimSpace(doc.Call("getElementById", "setup-username").Get("value").String())
		if username == "" {
			username = "root"
		}
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

	validateStep2 := func() bool {
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

	nextBtn.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) any {
		if currentStep == 1 {
			if validateStep1() {
				currentStep = 2
				updateUI()
			}
		} else if currentStep == 2 {
			if validateStep2() {
				currentStep = 3
				updateUI()
			}
		}
		return nil
	}))

	backBtn.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) any {
		if currentStep == 2 {
			currentStep = 1
			updateUI()
		} else if currentStep == 3 {
			currentStep = 2
			updateUI()
		}
		return nil
	}))

	form := doc.Call("getElementById", "setup-form")
	if form.Type() != js.TypeNull {
		form.Set("onsubmit", js.FuncOf(func(onSubmitThis js.Value, onSubmitArgs []js.Value) any {
			e := onSubmitArgs[0]
			e.Call("preventDefault")

			go func() {
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
			}()
			return nil
		}))
	}
	return nil
}

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

func logoutGo(this js.Value, args []js.Value) any {
	go func() {
		_, err := apiRequest("POST", "/logout", nil)
		if err != nil {
			fmt.Printf("Logout request failed: %v\n", err)
		}
		js.Global().Get("localStorage").Call("removeItem", "token")
		js.Global().Get("localStorage").Call("removeItem", "activeLibraryId")
		showLoginScreenGo(js.Undefined(), nil)
	}()
	return nil
}

func sanitizeHTMLGo(this js.Value, args []js.Value) any {
	if len(args) == 0 || args[0].Type() == js.TypeUndefined || args[0].Type() == js.TypeNull {
		return ""
	}
	html := args[0].String()
	if html == "" {
		return ""
	}

	parser := js.Global().Get("DOMParser").New()
	doc := parser.Call("parseFromString", html, "text/html")
	body := doc.Get("body")

	container := js.Global().Get("document").Call("createElement", "div")

	var sanitizeNode func(node js.Value) js.Value
	sanitizeNode = func(node js.Value) js.Value {
		nodeType := node.Get("nodeType").Int()
		if nodeType == 3 { // TEXT_NODE
			return node.Call("cloneNode", true)
		}
		if nodeType != 1 { // ELEMENT_NODE
			return js.Global().Get("document").Call("createTextNode", "")
		}

		tagName := node.Get("tagName").String()

		allowedTags := []string{"A", "B", "I", "U", "EM", "STRONG", "SPAN", "P", "BR", "DIV", "H1", "H2", "H3", "H4", "H5", "H6", "UL", "OL", "LI", "HR"}
		allowed := false
		for _, t := range allowedTags {
			if t == tagName {
				allowed = true
				break
			}
		}

		if !allowed {
			return js.Global().Get("document").Call("createTextNode", node.Get("textContent").String())
		}

		cleanEl := js.Global().Get("document").Call("createElement", tagName)

		allowedAttrs := map[string][]string{
			"A":    {"href", "target", "rel", "title"},
			"SPAN": {"class", "style"},
			"DIV":  {"class", "style"},
			"P":    {"class", "style"},
		}

		attrs := allowedAttrs[tagName]
		nodeAttrs := node.Get("attributes")
		for i := 0; i < nodeAttrs.Get("length").Int(); i++ {
			attr := nodeAttrs.Call("item", i)
			name := strings.ToLower(attr.Get("name").String())

			attrAllowed := false
			for _, a := range attrs {
				if a == name {
					attrAllowed = true
					break
				}
			}

			if attrAllowed {
				val := attr.Get("value").String()
				if name == "href" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(val)), "javascript:") {
					continue
				}
				cleanEl.Call("setAttribute", name, val)
			}
		}

		childNodes := node.Get("childNodes")
		for i := 0; i < childNodes.Get("length").Int(); i++ {
			cleanEl.Call("appendChild", sanitizeNode(childNodes.Call("item", i)))
		}

		return cleanEl
	}

	childNodes := body.Get("childNodes")
	for i := 0; i < childNodes.Get("length").Int(); i++ {
		container.Call("appendChild", sanitizeNode(childNodes.Call("item", i)))
	}

	return container.Get("innerHTML").String()
}
