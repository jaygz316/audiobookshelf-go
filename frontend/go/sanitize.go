//go:build js && wasm

package main

import (
	"strings"
	"syscall/js"
)

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
