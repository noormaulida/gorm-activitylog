package activitylog

import (
	"strings"
	"testing"
)

func TestEventHelpers(t *testing.T) {
	options := LogOptions{
		LogEvents: []string{" " + strings.ToUpper(EventUpdated) + " "},
		DescriptionForEvent: func(event string) string {
			return "custom " + event
		},
	}
	if !shouldLogEvent(options, EventUpdated) {
		t.Fatal("configured event was not matched")
	}
	if shouldLogEvent(options, EventCreated) {
		t.Fatal("unconfigured event was matched")
	}
	if got := eventDescription(options, EventUpdated); got != "custom updated" {
		t.Fatalf("unexpected event description: %q", got)
	}
	if got := eventDescription(LogOptions{}, EventDeleted); got != EventDeleted {
		t.Fatalf("unexpected default description: %q", got)
	}
}
