package dispatchers

// DispatchRecorder counts delivery outcomes. observability.Metrics satisfies it;
// defining the interface here keeps the dispatcher tests free of the platform
// module and lets a nil recorder mean "metrics disabled".
type DispatchRecorder interface {
	RecordDispatch(channel, status string)
}

func recordDispatch(recorder DispatchRecorder, channel, status string) {
	if recorder == nil {
		return
	}
	recorder.RecordDispatch(channel, status)
}
