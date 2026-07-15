package handlers

import (
	log "audiobookshelf/internal/logger"
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"

	"audiobookshelf/internal/core"
	"audiobookshelf/internal/podcast"
)

// handleGetTasks returns tasks list
func handleGetTasks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] GET /api/tasks")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		include := r.URL.Query().Get("include")
		includeArray := strings.Split(include, ",")

		var tasksList []map[string]interface{}
		if podcast.GlobalQueueManager != nil {
			tasksList = podcast.GlobalQueueManager.GetTasks()
		}

		data := map[string]interface{}{
			"tasks": tasksList,
		}

		hasQueue := false
		for _, inc := range includeArray {
			if inc == "queue" {
				hasQueue = true
				break
			}
		}

		if hasQueue {
			data["queuedTaskData"] = map[string]interface{}{
				"embedMetadata": []interface{}{},
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(data)
	}
}

// handleCancelAllTasks cancels all tasks
func handleCancelAllTasks(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/tasks/cancel-all")
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if podcast.GlobalQueueManager != nil {
			podcast.GlobalQueueManager.CancelAll()
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}

// handleSingleTaskAction executes cancel, pause, resume, or retry on a specific task
func handleSingleTaskAction(db *sql.DB, taskID, action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Infof("[Go] POST /api/tasks/%s/%s", taskID, action)
		userSess, ok := r.Context().Value(core.UserContextKey).(*core.UserSession)
		if !ok || !userSess.IsAdminOrUp() {
			http.Error(w, `{"error": "Forbidden"}`, http.StatusForbidden)
			return
		}

		if podcast.GlobalQueueManager != nil {
			switch action {
			case "cancel":
				podcast.GlobalQueueManager.Cancel(taskID)
			case "pause":
				podcast.GlobalQueueManager.Pause(taskID)
			case "resume", "retry":
				podcast.GlobalQueueManager.Resume(taskID)
			default:
				http.Error(w, `{"error": "Invalid action"}`, http.StatusBadRequest)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"success": true}`))
	}
}
