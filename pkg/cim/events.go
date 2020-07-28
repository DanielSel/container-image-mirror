package cim

import (
	"context"

	"github.com/go-logr/logr"
)

type Event struct {
	Type   EventType
	Source interface{}
	Data   interface{}
}

type EventType string

const (
	EventTypeStatusChange EventType = "StatusChange"
)

type EventHandler func(ctx context.Context, log logr.Logger, evt Event) error

func emit(ch chan Event, evt Event) {
	select {
	case ch <- evt:
	default:
	}
}
