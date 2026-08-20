/*
** Copyright (c) 2026 Oracle and/or its affiliates.
**
** The Universal Permissive License (UPL), Version 1.0
 */

package common

import "testing"

func TestSessionContext_UpdateSessionProperties_EmitsOnlyOnChange(t *testing.T) {
	t.Parallel()

	sessionCtx := NewSessionContext()
	sessionCtx.GetSessionProperties().SetProperty("foo", "bar")

	var notifications []struct {
		key      string
		oldValue any
		newValue any
	}
	sessionCtx.RegisterPropertyChangeListener(func(key string, oldValue any, newValue any) {
		notifications = append(notifications, struct {
			key      string
			oldValue any
			newValue any
		}{key: key, oldValue: oldValue, newValue: newValue})
	})

	unchanged := NewProperties[string]()
	unchanged.SetProperty("foo", "bar")
	sessionCtx.UpdateSessionProperties(unchanged)
	if len(notifications) != 0 {
		t.Fatalf("expected no notifications for unchanged property, got %d", len(notifications))
	}

	changed := NewProperties[string]()
	changed.SetProperty("foo", "baz")
	sessionCtx.UpdateSessionProperties(changed)
	if len(notifications) != 1 {
		t.Fatalf("expected 1 notification for changed property, got %d", len(notifications))
	}
	if notifications[0].key != "foo" || notifications[0].oldValue != "bar" || notifications[0].newValue != "baz" {
		t.Fatalf("unexpected notification payload: %+v", notifications[0])
	}
}
