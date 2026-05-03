package benchmarks

// Result is the persisted JSON schema for one benchmark run.
type Result struct {
	SchemaVersion int              `json:"schema_version"`
	Fixture       string           `json:"fixture"`
	GitSHA        string           `json:"git_sha"`
	GitDirty      bool             `json:"git_dirty"`
	Host          Host             `json:"host"`
	Engine        string           `json:"engine"`
	Threads       int              `json:"threads"`
	DurationMS    int64            `json:"duration_ms"`
	StagesMS      map[string]int64 `json:"stages_ms"`
	Throughput    Throughput       `json:"throughput"`
	RowsTotal     int64            `json:"rows_total"`
	BytesTotal    int64            `json:"bytes_total"`
	Tables        []TableResult    `json:"tables"`
	Memory        MemoryUsage      `json:"memory"`
}

// Host captures machine metadata for a benchmark run.
type Host struct {
	OS    string `json:"os"`
	Arch  string `json:"arch"`
	CPU   string `json:"cpu"`
	Cores int    `json:"cores"`
	RAMGB int    `json:"ram_gb"`
}

// Throughput summarizes rows and bytes processed per second.
type Throughput struct {
	RowsPerSec  float64 `json:"rows_per_sec"`
	BytesPerSec float64 `json:"bytes_per_sec"`
}

// TableResult records per-table benchmark measurements.
type TableResult struct {
	Name    string `json:"name"`
	Rows    int64  `json:"rows"`
	Bytes   int64  `json:"bytes"`
	Seconds int64  `json:"seconds"`
}

// MemoryUsage records peak process memory observed during a benchmark.
type MemoryUsage struct {
	PeakRSSBytes uint64 `json:"peak_rss_bytes"`
}

// Validate checks that the benchmark result contains required fields.
func (r Result) Validate() error {
	if r.SchemaVersion != 1 {
		return errString("schema_version must be 1")
	}
	if r.Fixture == "" || r.Engine == "" || r.DurationMS <= 0 {
		return errString("fixture, engine, and positive duration_ms are required")
	}
	return nil
}

type errString string

func (e errString) Error() string { return string(e) }
