package event

import (
	"sync"
)

type MockRecorder struct {
	mu     sync.Mutex
	events []Event
}

func NewMockRecorder() *MockRecorder {
	return &MockRecorder{
		events: []Event{},
	}
}

func (r *MockRecorder) Publish(event ...Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, event...)
}

func (r *MockRecorder) Calls() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events
}

func (r *MockRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = []Event{}
}
