package global

type WorkerStats struct {
	Claimed     int
	Completed   int
	Failed      int
	Retried     int
	Interrupted int
	LeaseLost   int
}

type JobProgress struct {
	JobID   string
	Total   int
	Pending int
	Leased  int
	Done    int
	Failed  int
}

type Rendition struct {
	Name         string
	Height       int
	VideoBitrate int
	AudioBitrate int
}
