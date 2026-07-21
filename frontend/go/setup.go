//go:build js && wasm

package main

import (
	"strings"
	"syscall/js"
)

func showSetupScreenGo(this js.Value, args []js.Value) any {
	doc := js.Global().Get("document")
	if setupScreen := doc.Call("getElementById", "setup-screen"); setupScreen.Type() != js.TypeNull && setupScreen.Type() != js.TypeUndefined {
		setupScreen.Get("classList").Call("remove", "hidden")
	}
	if loginScreen := doc.Call("getElementById", "login-screen"); loginScreen.Type() != js.TypeNull && loginScreen.Type() != js.TypeUndefined {
		loginScreen.Get("classList").Call("add", "hidden")
	}
	if appContainer := doc.Call("getElementById", "app-container"); appContainer.Type() != js.TypeNull && appContainer.Type() != js.TypeUndefined {
		appContainer.Get("classList").Call("add", "hidden")
	}

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

	nextBtn.Set("onclick", js.FuncOf(func(this js.Value, args []js.Value) any {
		if currentStep == 1 {
			if validateStep1(doc, errEl) {
				currentStep = 2
				updateUI()
			}
		} else if currentStep == 2 {
			if validateStep2(doc, errEl) {
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

			go submitSetupForm(doc, errEl, successEl, submitBtn)
			return nil
		}))
	}
	return nil
}
