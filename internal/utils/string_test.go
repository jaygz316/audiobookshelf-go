package utils

import (
	"testing"
)

func TestNormalizeTitleForSeries(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"The Hobbit", "the hobbit"},
		{"The Hobbit (Narrated by Andy Serkis)", "the hobbit"},
		{"The Hobbit [Unabridged]", "the hobbit"},
		{"The Hobbit - Read by Rob Inglis", "the hobbit"},
		{"The Hobbit - Special Edition", "the hobbit"},
		{"The Fellowship of the Ring (Book 1 of Lord of the Rings)", "the fellowship of the ring book 1 of lord of the rings"},
		{"Harry Potter and the Sorcerer's Stone: Unabridged Audiobook", "harry potter and the sorcerer's stone"},
		{"  The Hobbit  ", "the hobbit"},
		{"Aversion Therapy", "aversion therapy"},
		{"Aversion Therapy - Aversion", "aversion therapy"},
		{"The Hobbit (Narrated by Andy Serkis) [Unabridged]", "the hobbit"},
		{"Unbalanced (Parenthesis", "unbalanced parenthesis"},
	}

	for _, tt := range tests {
		got := NormalizeTitleForSeries(tt.input)
		if got != tt.expected {
			t.Errorf("NormalizeTitleForSeries(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNameToLastFirst(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"John Doe", "Doe, John"},
		{"John", "John"},
		{"", ""},
		{"John Middle Doe", "Doe, John Middle"},
		{"  John   Doe  ", "Doe, John"},
	}

	for _, tt := range tests {
		got := NameToLastFirst(tt.input)
		if got != tt.expected {
			t.Errorf("NameToLastFirst(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}

func TestNullIfEmpty(t *testing.T) {
	tests := []struct {
		input    string
		expected *string
	}{
		{"", nil},
		{"hello", ptr("hello")},
	}

	for _, tt := range tests {
		got := NullIfEmpty(tt.input)
		if got == nil && tt.expected != nil {
			t.Errorf("NullIfEmpty(%q) = nil, want %q", tt.input, *tt.expected)
		} else if got != nil && tt.expected == nil {
			t.Errorf("NullIfEmpty(%q) = %q, want nil", tt.input, *got)
		} else if got != nil && tt.expected != nil && *got != *tt.expected {
			t.Errorf("NullIfEmpty(%q) = %q, want %q", tt.input, *got, *tt.expected)
		}
	}
}

func ptr(s string) *string {
	return &s
}
