package activitylog

import "strings"

const (
	// EventCreated is emitted after a model is inserted.
	EventCreated = "created"
	// EventUpdated is emitted after a model is updated.
	EventUpdated = "updated"
	// EventDeleted is emitted after a model is soft-deleted or hard-deleted.
	EventDeleted = "deleted"
	// EventRestored is emitted when a soft-deleted model is restored.
	EventRestored = "restored"
)

func shouldLogEvent(options LogOptions, event string) bool {
	if len(options.LogEvents) == 0 {
		return true
	}

	for _, configured := range options.LogEvents {
		if strings.EqualFold(strings.TrimSpace(configured), event) {
			return true
		}
	}
	return false
}

func eventDescription(options LogOptions, event string) string {
	if options.DescriptionForEvent == nil {
		return event
	}
	return options.DescriptionForEvent(event)
}
