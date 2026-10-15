# golden-signals

View latency percentiles, request rate, errors, and saturation in a local dashboard built from JSON Lines events.

## Install

With Go 1.23 or newer, install it with `go install github.com/ulrichheringer/golden-signals@latest`.

## Use

Each event needs `timestamp`, `latency_ms`, and `status`; `cpu_pct`, `memory_bytes`, and `queue_depth` are optional. Start the dashboard with `golden-signals -input events.jsonl`.

## Example

The example data is simulated. Run `go run . -input examples/simulated-events.jsonl`, then open the URL printed in the terminal.
