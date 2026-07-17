//go:build js && wasm

package main

import (
	"encoding/json"
	"fmt"
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
