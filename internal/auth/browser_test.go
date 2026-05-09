package auth

import "testing"

func TestNewBrowserAuthUsesLoopbackCompatibleClientByDefault(t *testing.T) {
	b, err := NewBrowserAuth("", []string{"Calendars.Read"})
	if err != nil {
		t.Fatalf("NewBrowserAuth() error = %v", err)
	}

	if b.clientID != DefaultBrowserClientID {
		t.Fatalf("NewBrowserAuth() clientID = %q, want %q", b.clientID, DefaultBrowserClientID)
	}
}
