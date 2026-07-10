package backup

import (
	"testing"
	"time"
)

func TestMatchCron(t *testing.T) {
	tests := []struct {
		name       string
		expression string
		t          time.Time
		expected   bool
	}{
		{
			name:       "All wildcards 5 fields",
			expression: "* * * * *",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC),
			expected:   true,
		},
		{
			name:       "All wildcards 6 fields",
			expression: "* * * * * *",
			t:          time.Date(2026, 7, 10, 12, 34, 56, 0, time.UTC),
			expected:   true,
		},
		{
			name:       "Specific minute match",
			expression: "34 * * * *",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC),
			expected:   true,
		},
		{
			name:       "Specific minute mismatch",
			expression: "35 * * * *",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC),
			expected:   false,
		},
		{
			name:       "Step divisor match",
			expression: "*/5 * * * *",
			t:          time.Date(2026, 7, 10, 12, 35, 0, 0, time.UTC),
			expected:   true,
		},
		{
			name:       "Step divisor mismatch",
			expression: "*/5 * * * *",
			t:          time.Date(2026, 7, 10, 12, 37, 0, 0, time.UTC),
			expected:   false,
		},
		{
			name:       "Comma separated list match",
			expression: "1,2,34,50 * * * *",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC),
			expected:   true,
		},
		{
			name:       "Range match",
			expression: "30-40 * * * *",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC),
			expected:   true,
		},
		{
			name:       "Range mismatch",
			expression: "30-40 * * * *",
			t:          time.Date(2026, 7, 10, 12, 45, 0, 0, time.UTC),
			expected:   false,
		},
		{
			name:       "Day of week match (Friday = 5)",
			expression: "* * * * 5",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC), // 2026-07-10 is a Friday
			expected:   true,
		},
		{
			name:       "Day of week mismatch (Friday = 5, expected 6)",
			expression: "* * * * 6",
			t:          time.Date(2026, 7, 10, 12, 34, 0, 0, time.UTC),
			expected:   false,
		},
		{
			name:       "Day of week Sunday (7 or 0)",
			expression: "* * * * 7",
			t:          time.Date(2026, 7, 12, 12, 34, 0, 0, time.UTC), // 2026-07-12 is a Sunday
			expected:   true,
		},
		{
			name:       "Complex cron match",
			expression: "0 12 */2 * *",
			t:          time.Date(2026, 7, 10, 12, 0, 0, 0, time.UTC), // 10th of month (divisible by 2) at 12:00
			expected:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := MatchCron(tc.expression, tc.t)
			if got != tc.expected {
				t.Errorf("MatchCron(%q, %v) = %v; expected %v", tc.expression, tc.t, got, tc.expected)
			}
		})
	}
}
