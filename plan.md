# Implementation Plan: Scheduled Metadata Backups

We will implement automated, scheduled metadata backups to achieve parity with Audiobookshelf's backup system.

## Proposed Changes

1. **Database Schema & ServerSettings Update**
   - Add `BackupSchedule` string field to `ServerSettings` struct in [db.go](file:///home/jay/projects/audiobookshelf-go/internal/db/db.go).
   - Set up default backup schedule to hourly, daily, or empty (disabled).

2. **Cron Scheduler Engine**
   - Create `internal/backup/scheduler.go` containing:
     - Cron expression parser supporting 5-field and 6-field formats.
     - Matcher to check if a specific time matches the parsed cron expression.
     - Background runner (`BackupScheduler`) checking settings every minute.
     - Logic to trigger backup and prune older backups based on settings.
   - Add thread-safe start, stop, and restart controls for the scheduler.

3. **Lifecycle Integration**
   - Start the scheduler in `main.go` after database initialization.
   - Restart/reload the scheduler when server settings are updated via `PATCH /api/settings` in [settings_server.go](file:///home/jay/projects/audiobookshelf-go/internal/handlers/settings_server.go).
   - Gracefully stop the scheduler on server shutdown.

4. **Testing**
   - Unit tests in `internal/backup/scheduler_test.go` verifying the cron parser/matcher.
   - Integration tests verifying backup triggers.
