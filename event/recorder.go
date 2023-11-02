package event

import (
	"fmt"
	"strings"
	"time"

	"github.com/patrickmn/go-cache"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/flowcontrol"
)

type Recorder interface {
	Publish(...Event)
}

type Event struct {
	InvolvedObject runtime.Object
	Type           string
	Reason         string
	Message        string
	DedupeValues   []string
	DedupeTimeout  time.Duration
	RateLimiter    flowcontrol.RateLimiter
}

func (e Event) dedupeKey() string {
	return fmt.Sprintf("%s-%s",
		strings.ToLower(e.Reason),
		strings.Join(e.DedupeValues, "-"),
	)
}

type recorder struct {
	record.EventRecorder
	cache *cache.Cache
}

const defaultDedupeTimeout = 2 * time.Minute

func NewRecorder(r record.EventRecorder) Recorder {
	return &recorder{
		EventRecorder: r,
		cache:         cache.New(defaultDedupeTimeout, 10*time.Second),
	}
}

// Publish creates a Kubernetes event using the passed event struct
func (r *recorder) Publish(evts ...Event) {
	for _, evt := range evts {
		r.publish(evt)
	}
}

func (r *recorder) publish(evt Event) {
	// Override the timeout if one is set for an event
	timeout := defaultDedupeTimeout
	if evt.DedupeTimeout != 0 {
		timeout = evt.DedupeTimeout
	}
	// Dedupe same events that involve the same object and are close together
	if len(evt.DedupeValues) > 0 && !r.shouldCreateEvent(evt.dedupeKey(), timeout) {
		return
	}
	// If the event is rate-limited, then validate we should create the event
	if evt.RateLimiter != nil && !evt.RateLimiter.TryAccept() {
		return
	}
	r.Event(evt.InvolvedObject, evt.Type, evt.Reason, evt.Message)
}

func (r *recorder) shouldCreateEvent(key string, timeout time.Duration) bool {
	if _, exists := r.cache.Get(key); exists {
		return false
	}
	r.cache.Set(key, nil, timeout)
	return true
}
