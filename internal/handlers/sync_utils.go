package handlers

import (
	"fmt"
	"strconv"
	"time"
)

func parseJSONTime(val interface{}) time.Time {
	if val == nil {
		return time.Now()
	}
	switch v := val.(type) {
	case float64:
		return time.UnixMilli(int64(v))
	case int64:
		return time.UnixMilli(v)
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02 15:04:05.999 Z07:00", v); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02 15:04:05.999", v); err == nil {
			return t
		}
		if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
			return time.UnixMilli(ms)
		}
	}
	return time.Now()
}

func stringifyDeviceInfo(val interface{}) string {
	if val == nil {
		return "Unknown Device"
	}
	switch v := val.(type) {
	case string:
		return v
	case map[string]interface{}:
		clientName := ""
		if cn, ok := v["clientName"].(string); ok {
			clientName = cn
		} else if bn, ok := v["browserName"].(string); ok {
			clientName = bn
		}

		osName := ""
		if os, ok := v["osName"].(string); ok {
			osName = os
		}

		if clientName != "" && osName != "" {
			return clientName + " / " + osName
		} else if clientName != "" {
			return clientName
		} else if osName != "" {
			return osName
		}
	}
	return "Unknown Device"
}

func stringifyPlayMethod(val interface{}) string {
	if val == nil {
		return "HLS"
	}
	switch v := val.(type) {
	case string:
		return v
	case float64:
		switch int(v) {
		case 0:
			return "Direct Play"
		case 1:
			return "Direct Stream"
		case 2:
			return "Transcode"
		}
		return fmt.Sprintf("PlayMethod %d", int(v))
	}
	return "HLS"
}
