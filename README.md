# golden-signals

Serve a local dashboard for latency percentiles, request traffic, 5xx errors, and saturation from JSON-lines event data.

## Install

Requires Go 1.23 or newer. Build with `go install github.com/ulrichheringer/golden-signals@latest`.

## Use

Provide `timestamp`, `latency_ms`, `status`, and optional `cpu_pct`, `memory_bytes`, and `queue_depth` fields. Run `golden-signals -input events.jsonl`.

## Example

Run `go run . -input examples/simulated-events.jsonl` and open the printed local URL. The example data is simulated.
