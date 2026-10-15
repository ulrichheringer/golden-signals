package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"sort"
	"time"
)

type Sample struct {
	Timestamp    time.Time `json:"-"`
	TimestampRaw string    `json:"timestamp"`
	LatencyMS    float64   `json:"latency_ms"`
	Status       int       `json:"status"`
	CPUPct       float64   `json:"cpu_pct"`
	MemoryBytes  int64     `json:"memory_bytes"`
	QueueDepth   float64   `json:"queue_depth"`
}

type Metrics struct {
	Samples           int     `json:"samples"`
	RequestsPerSecond float64 `json:"requests_per_second"`
	ErrorCount        int     `json:"error_count"`
	ErrorRate         float64 `json:"error_rate"`
	P50MS             float64 `json:"p50_ms"`
	P90MS             float64 `json:"p90_ms"`
	P99MS             float64 `json:"p99_ms"`
	AverageCPU        float64 `json:"average_cpu_pct"`
	MaxCPU            float64 `json:"max_cpu_pct"`
	MaxMemoryBytes    int64   `json:"max_memory_bytes"`
	MaxQueueDepth     float64 `json:"max_queue_depth"`
	HasCPU            bool    `json:"has_cpu"`
	HasMemory         bool    `json:"has_memory"`
	HasQueue          bool    `json:"has_queue"`
}

func ReadSamples(r io.Reader) ([]Sample, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	var samples []Sample
	for line := 1; scanner.Scan(); line++ {
		var sample Sample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		if sample.TimestampRaw != "" {
			parsed, err := time.Parse(time.RFC3339Nano, sample.TimestampRaw)
			if err != nil {
				return nil, fmt.Errorf("line %d timestamp: %w", line, err)
			}
			sample.Timestamp = parsed
		}
		samples = append(samples, sample)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func Calculate(samples []Sample) Metrics {
	m := Metrics{Samples: len(samples)}
	if len(samples) == 0 {
		return m
	}
	var latencies []float64
	var first, last time.Time
	cpuCount := 0
	for _, sample := range samples {
		latencies = append(latencies, sample.LatencyMS)
		if sample.Status >= 500 {
			m.ErrorCount++
		}
		if sample.CPUPct != 0 {
			m.HasCPU = true
			m.AverageCPU += sample.CPUPct
			cpuCount++
			if sample.CPUPct > m.MaxCPU {
				m.MaxCPU = sample.CPUPct
			}
		}
		if sample.MemoryBytes > 0 {
			m.HasMemory = true
			if sample.MemoryBytes > m.MaxMemoryBytes {
				m.MaxMemoryBytes = sample.MemoryBytes
			}
		}
		if sample.QueueDepth != 0 {
			m.HasQueue = true
			if sample.QueueDepth > m.MaxQueueDepth {
				m.MaxQueueDepth = sample.QueueDepth
			}
		}
		if !sample.Timestamp.IsZero() {
			if first.IsZero() || sample.Timestamp.Before(first) {
				first = sample.Timestamp
			}
			if last.IsZero() || sample.Timestamp.After(last) {
				last = sample.Timestamp
			}
		}
	}
	sort.Float64s(latencies)
	m.P50MS, m.P90MS, m.P99MS = percentile(latencies, .50), percentile(latencies, .90), percentile(latencies, .99)
	if len(samples) > 0 {
		m.ErrorRate = float64(m.ErrorCount) / float64(len(samples))
	}
	if cpuCount > 0 {
		m.AverageCPU /= float64(cpuCount)
	}
	if !first.IsZero() && last.After(first) {
		m.RequestsPerSecond = float64(len(samples)) / last.Sub(first).Seconds()
	}
	return m
}

func percentile(sorted []float64, fraction float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	index := int(fraction*float64(len(sorted))+0.999999) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

const page = `<!doctype html><html><head><meta charset="utf-8"><title>Golden Signals</title><style>body{font:16px system-ui;max-width:900px;margin:3rem auto;padding:0 1rem;background:#f5f7fa;color:#17212b}h1{margin-bottom:.3rem}.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(210px,1fr));gap:1rem}.card{background:white;border:1px solid #dce3ea;border-radius:10px;padding:1rem}.label{color:#566574;font-size:.9rem}.value{font-size:1.8rem;font-weight:650;margin-top:.4rem}small{color:#667}</style></head><body><h1>Golden Signals</h1><p>Snapshot from {{.Samples}} input events</p><div class="grid"><div class="card"><div class="label">Latency</div><div class="value">p50 {{printf "%.1f" .P50MS}} ms</div><p>p90 {{printf "%.1f" .P90MS}} ms · p99 {{printf "%.1f" .P99MS}} ms</p></div><div class="card"><div class="label">Traffic</div><div class="value">{{printf "%.2f" .RequestsPerSecond}} req/s</div><p>{{.Samples}} total requests</p></div><div class="card"><div class="label">Errors</div><div class="value">{{printf "%.2f" (mul100 .ErrorRate)}}%</div><p>{{.ErrorCount}} responses with status 500+</p></div><div class="card"><div class="label">Saturation</div><div class="value">{{if .HasCPU}}{{printf "%.1f" .MaxCPU}}% CPU{{else}}CPU n/a{{end}}</div><p>{{if .HasMemory}}max memory {{.MaxMemoryBytes}} bytes{{else}}Memory n/a{{end}}{{if .HasQueue}} · max queue {{printf "%.1f" .MaxQueueDepth}}{{end}}</p></div></div><p><a href="/metrics.json">View metrics JSON</a></p><small>Traffic rate needs timestamps spanning more than one instant. Missing saturation fields are shown as n/a.</small></body></html>`

func handler(metrics Metrics) http.Handler {
	tmpl := template.Must(template.New("dashboard").Funcs(template.FuncMap{"mul100": func(v float64) float64 { return v * 100 }}).Parse(page))
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = tmpl.Execute(w, metrics)
	})
	mux.HandleFunc("/metrics.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(metrics)
	})
	return mux
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	flags := flag.NewFlagSet("golden-signals", flag.ContinueOnError)
	flags.SetOutput(stderr)
	input := flags.String("input", "", "JSON-lines events file")
	addr := flags.String("addr", "127.0.0.1:8080", "local dashboard address")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *input == "" {
		return errors.New("usage: golden-signals -input events.jsonl [-addr 127.0.0.1:8080]")
	}
	file, err := os.Open(*input)
	if err != nil {
		return err
	}
	defer file.Close()
	samples, err := ReadSamples(file)
	if err != nil {
		return err
	}
	metrics := Calculate(samples)
	server := &http.Server{Addr: *addr, Handler: handler(metrics), ReadHeaderTimeout: 5 * time.Second}
	if _, err := fmt.Fprintf(stdout, "Golden Signals dashboard: http://%s\n", *addr); err != nil {
		return err
	}
	return server.ListenAndServe()
}
