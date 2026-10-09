package activitylog

import "strings"

const (
	// EventCreated is emitted after a model is inserted.
	EventCreated = "created"
	// EventUpdated is emitted after a model is updated.
	EventUpdated = "updated"
	// EventDeleted is emitted after a model is deleted.
	EventDeleted = "deleted"
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
