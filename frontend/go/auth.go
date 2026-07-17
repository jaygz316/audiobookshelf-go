//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
	"syscall/js"
)

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
