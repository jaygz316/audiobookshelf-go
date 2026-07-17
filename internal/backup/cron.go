package backup

import (
	"strconv"
	"strings"
	"time"
)

// MatchCron evaluates if the given time matches the cron expression.
func MatchCron(expression string, t time.Time) bool {
	parts := strings.Fields(expression)
	if len(parts) != 5 && len(parts) != 6 {
		return false
	}

	var secPart, minPart, hourPart, domPart, monthPart, dowPart string
	if len(parts) == 5 {
		secPart = "0"
		minPart = parts[0]
		hourPart = parts[1]
		domPart = parts[2]
		monthPart = parts[3]
		dowPart = parts[4]
	} else {
		secPart = parts[0]
		minPart = parts[1]
		hourPart = parts[2]
		domPart = parts[3]
		monthPart = parts[4]
		dowPart = parts[5]
	}

	if !matchCronField(secPart, t.Second()) {
		return false
	}
	if !matchCronField(minPart, t.Minute()) {
		return false
	}
	if !matchCronField(hourPart, t.Hour()) {
		return false
	}
	if !matchCronField(domPart, t.Day()) {
		return false
	}
	if !matchCronField(monthPart, int(t.Month())) {
		return false
	}

	dowVal := int(t.Weekday())
	if dowPart != "*" {
		dowPartNormalized := strings.ReplaceAll(dowPart, "7", "0")
		if !matchCronField(dowPartNormalized, dowVal) {
			return false
		}
	}

	return true
}

func matchCronField(field string, value int) bool {
	if field == "*" {
		return true
	}
	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			if matchCronField(part, value) {
				return true
			}
		}
		return false
	}
	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return false
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil {
			return false
		}
		rangePart := parts[0]
		if rangePart == "*" {
			return value%step == 0
		}
		start, end, ok := parseCronRange(rangePart)
		if !ok {
			return false
		}
		return value >= start && value <= end && (value-start)%step == 0
	}
	if strings.Contains(field, "-") {
		start, end, ok := parseCronRange(field)
		if !ok {
			return false
		}
		return value >= start && value <= end
	}
	val, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	return val == value
}

func parseCronRange(field string) (int, int, bool) {
	parts := strings.Split(field, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err1 := strconv.Atoi(parts[0])
	end, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return start, end, true
}
