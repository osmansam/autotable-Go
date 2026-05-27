package events

import "github.com/osmansam/autotableGo/ws"

type DynamicEvents struct{}

func NewDynamicEvents() *DynamicEvents {
	return &DynamicEvents{}
}

func (e *DynamicEvents) EmitInvalidate(schemaName, userID, tenantID, projectID string, eventID ...string) {
	ws.EmitInvalidate(schemaName, userID, tenantID, projectID, eventID...)
}
