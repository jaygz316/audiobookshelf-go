package podcast

import (
	"context"
	"strconv"
	"strings"
	"time"

	log "audiobookshelf/internal/logger"
)

// ScheduleRefresh initiates recurring background ticks for feed synchronization.
func (m *PodcastManager) ScheduleRefresh(ctx context.Context, cronExpression string) error {
	duration := parseCronToDuration(cronExpression)

	go func() {
		// Run once immediately
		if err := m.SyncAllFeeds(ctx); err != nil {
			// PORT: Sync error during tick is logged/ignored to not crash the scheduler.
			log.Printf("[Podcast] Sync error during initial run: %v", err)
		}

		ticker := time.NewTicker(duration)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.SyncAllFeeds(ctx); err != nil {
					// PORT: Sync error during tick is logged/ignored to not crash the scheduler.
					log.Printf("[Podcast] Sync error during tick: %v", err)
				}
			}
		}
	}()

	return nil
}

func parseCronToDuration(expr string) time.Duration {
	parts := strings.Fields(expr)
	if len(parts) < 5 {
		return 1 * time.Hour
	}

	minPart := parts[0]
	hourPart := parts[1]

	if minPart == "*" {
		return 1 * time.Minute
	}
	if strings.HasPrefix(minPart, "*/") {
		if val, err := strconv.Atoi(minPart[2:]); err == nil {
			return time.Duration(val) * time.Minute
		}
	}

	if hourPart == "*" {
		return 1 * time.Hour
	}
	if strings.HasPrefix(hourPart, "*/") {
		if val, err := strconv.Atoi(hourPart[2:]); err == nil {
			return time.Duration(val) * time.Hour
		}
	}

	if hourPart != "*" && minPart != "*" {
		return 24 * time.Hour
	}

	return 1 * time.Hour
}
