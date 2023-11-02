package event

type noopRecorder struct{}

func (*noopRecorder) Publish(...Event) {}
