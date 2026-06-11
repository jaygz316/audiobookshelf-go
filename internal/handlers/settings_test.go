package handlers

import (
	"reflect"
	"testing"
)

func TestGetTitleIgnorePrefixGo(t *testing.T) {
	prefixes := []string{"the", "a", "an"}
	tests := []struct {
		title    string
		expected string
	}{
		{"The Hobbit", "Hobbit"},
		{"A Tale of Two Cities", "Tale of Two Cities"},
		{"An Elephant in the Room", "Elephant in the Room"},
		{"Another Title", "Another Title"},
		{"Theatre of Dreams", "Theatre of Dreams"}, // prefix matches substring but not followed by space
		{"the matrix", "matrix"},
	}

	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			actual := getTitleIgnorePrefixGo(tt.title, prefixes)
			if actual != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, actual)
			}
		})
	}
}

func TestCleanSortingPrefixes(t *testing.T) {
	input := []string{" The", "a ", "  AN  ", "the", "", "  "}
	expected := []string{"the", "a", "an"}

	actual := cleanSortingPrefixes(input)
	if !reflect.DeepEqual(actual, expected) {
		t.Errorf("expected %v, got %v", expected, actual)
	}
}

func TestSanitizeBrowserSettings(t *testing.T) {
	input := map[string]interface{}{
		"tokenSecret":                  "supersecret",
		"authOpenIDClientID":           "client-123",
		"authOpenIDClientSecret":       "client-secret-xyz",
		"authOpenIDMobileRedirectURIs": []string{"redirect-uri"},
		"authOpenIDGroupClaim":         "groups",
		"language":                     "",
		"authActiveAuthMethods":        nil,
		"allowedKey":                   "value",
	}

	sanitized := sanitizeBrowserSettings(input)

	// Verify secrets are deleted
	secrets := []string{
		"tokenSecret", "authOpenIDClientID", "authOpenIDClientSecret",
		"authOpenIDMobileRedirectURIs", "authOpenIDGroupClaim", "authOpenIDAdvancedPermsClaim",
	}
	for _, sec := range secrets {
		if _, exists := sanitized[sec]; exists {
			t.Errorf("secret key %q should be sanitized out of browser settings", sec)
		}
	}

	// Verify allowed fields exist
	if sanitized["allowedKey"] != "value" {
		t.Errorf("expected allowedKey to be 'value', got %v", sanitized["allowedKey"])
	}

	// Verify essential defaults are populated
	if sanitized["language"] != "en-us" {
		t.Errorf("expected language to default to 'en-us', got %v", sanitized["language"])
	}
	if !reflect.DeepEqual(sanitized["authActiveAuthMethods"], []string{"local"}) {
		t.Errorf("expected authActiveAuthMethods to default to ['local'], got %v", sanitized["authActiveAuthMethods"])
	}
}

func TestBuildAuthSettingsResponse(t *testing.T) {
	input := map[string]interface{}{
		"authLoginCustomMessage": "welcome",
		"authOpenIDIssuerURL":    "https://issuer.com",
	}

	authResponse := buildAuthSettingsResponse(input)

	// Check mapped fields
	if authResponse["authLoginCustomMessage"] != "welcome" {
		t.Errorf("expected authLoginCustomMessage to be 'welcome'")
	}
	if authResponse["authOpenIDIssuerURL"] != "https://issuer.com" {
		t.Errorf("expected authOpenIDIssuerURL to be 'https://issuer.com'")
	}

	// Check default values
	if authResponse["authOpenIDButtonText"] != "Login with OpenId" {
		t.Errorf("expected default button text")
	}
	if authResponse["authOpenIDTokenSigningAlgorithm"] != "RS256" {
		t.Errorf("expected default signing algorithm")
	}
	if !reflect.DeepEqual(authResponse["authOpenIDMobileRedirectURIs"], []string{"audiobookshelf://oauth"}) {
		t.Errorf("expected default mobile redirect URIs")
	}

	// Check sample permissions
	samplePerms, ok := authResponse["authOpenIDSamplePermissions"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected authOpenIDSamplePermissions map")
	}
	if samplePerms["download"] != true || samplePerms["accessExplicitContent"] != false {
		t.Errorf("sample permissions mismatch")
	}
}

func TestBuildMetadataProvidersResponse(t *testing.T) {
	customBooks := []map[string]interface{}{
		{"value": "custom-book-1", "text": "My Book Provider"},
	}
	customPodcasts := []map[string]interface{}{
		{"value": "custom-pod-1", "text": "My Pod Provider"},
	}

	response := buildMetadataProvidersResponse(customBooks, customPodcasts)
	providers, ok := response["providers"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected providers map")
	}

	books, ok := providers["books"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected books list")
	}
	podcasts, ok := providers["podcasts"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected podcasts list")
	}

	// Check that builtins are present
	foundGoogle := false
	for _, b := range books {
		if b["value"] == "google" {
			foundGoogle = true
			break
		}
	}
	if !foundGoogle {
		t.Errorf("expected google builtin in book providers")
	}

	// Check that custom is appended
	foundCustom := false
	for _, b := range books {
		if b["value"] == "custom-book-1" {
			foundCustom = true
		}
	}
	if !foundCustom {
		t.Errorf("expected custom-book-1 in book providers")
	}

	foundCustomPod := false
	for _, p := range podcasts {
		if p["value"] == "custom-pod-1" {
			foundCustomPod = true
		}
	}
	if !foundCustomPod {
		t.Errorf("expected custom-pod-1 in podcast providers")
	}
}
