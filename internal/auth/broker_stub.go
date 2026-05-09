//go:build !linux

package auth

import (
	"context"
	"errors"
)

var ErrBrokerNotAvailable = errors.New("microsoft identity broker not available")

// Broker is unavailable on non-Linux platforms.
type Broker struct{}

// NewBroker returns an unavailable broker on non-Linux platforms.
func NewBroker(clientID string, scopes []string) *Broker {
	return &Broker{}
}

// IsAvailable reports that the D-Bus broker is unavailable.
func (b *Broker) IsAvailable(ctx context.Context) bool {
	return false
}

// GetToken always fails because the D-Bus broker is unavailable.
func (b *Broker) GetToken(ctx context.Context) (*Token, error) {
	return nil, ErrBrokerNotAvailable
}

// Close is a no-op for the unavailable broker.
func (b *Broker) Close() error {
	return nil
}
