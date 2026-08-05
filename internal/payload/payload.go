// Package payload decodes the JSON session payload Claude Code writes to the
// status line's stdin.
//
// Two rules shape every type here.
//
// First, absence is never zero. Every leaf is a pointer and every reader is an
// accessor returning (value, ok), because a missing `cost` and a genuinely free
// session must not render identically. PRD §3.1.2 records that the likeliest
// failure over twelve months is a payload schema change, and untyped zero
// values would turn that into a segment that quietly disappears.
//
// Second, decoding never fails the render. Decode always returns a usable
// *Payload; the error is advisory, for `doctor`. PRD §3.3 requires a line on
// stdout for any input at all, including none.
package payload

import (
	"encoding/json"
	"errors"
	"io"
)

// maxPayloadBytes caps what Decode will read. Real payloads are a few KB; the
// limit exists so a wedged or hostile writer on stdin cannot grow the render
// path's memory without bound.
const maxPayloadBytes = 4 << 20 // 4 MiB

// Payload is one status line invocation's session state. The zero value is
// valid and reports every field absent.
type Payload struct {
	Model         *Model         `json:"model"`
	Workspace     *Workspace     `json:"workspace"`
	Cost          *Cost          `json:"cost"`
	ContextWindow *ContextWindow `json:"context_window"`
	RateLimits    *RateLimits    `json:"rate_limits"`

	SessionID       *string `json:"session_id"`
	SessionName     *string `json:"session_name"`
	Version         *string `json:"version"`
	TranscriptPath  *string `json:"transcript_path"`
	Cwd             *string `json:"cwd"`
	Exceeds200kToks *bool   `json:"exceeds_200k_tokens"`

	// raw is the undecoded input, kept for KeyDiff. Nil when nothing decoded.
	raw []byte
}

type Model struct {
	ID          *string `json:"id"`
	DisplayName *string `json:"display_name"`
}

type Workspace struct {
	CurrentDir  *string  `json:"current_dir"`
	ProjectDir  *string  `json:"project_dir"`
	AddedDirs   []string `json:"added_dirs"`
	GitWorktree *string  `json:"git_worktree"`
	Repo        *Repo    `json:"repo"`
}

type Repo struct {
	Host  *string `json:"host"`
	Name  *string `json:"name"`
	Owner *string `json:"owner"`
}

// Cost's numerics are float64 even where the value is conceptually an integer.
// JSON numbers decode to float64 anyway, and a payload that ever writes 1e6
// would fail to unmarshal into an int64 — failing the whole struct, not one
// field. Every quantity here is far below 2^53, so nothing is lost.
type Cost struct {
	TotalCostUSD      *float64 `json:"total_cost_usd"`
	TotalDurationMS   *float64 `json:"total_duration_ms"`
	TotalAPIDuration  *float64 `json:"total_api_duration_ms"`
	TotalLinesAdded   *float64 `json:"total_lines_added"`
	TotalLinesRemoved *float64 `json:"total_lines_removed"`
}

type ContextWindow struct {
	TotalInputTokens    *float64      `json:"total_input_tokens"`
	TotalOutputTokens   *float64      `json:"total_output_tokens"`
	ContextWindowSize   *float64      `json:"context_window_size"`
	UsedPercentage      *float64      `json:"used_percentage"`
	RemainingPercentage *float64      `json:"remaining_percentage"`
	CurrentUsage        *CurrentUsage `json:"current_usage"`
}

type CurrentUsage struct {
	InputTokens              *float64 `json:"input_tokens"`
	CacheReadInputTokens     *float64 `json:"cache_read_input_tokens"`
	CacheCreationInputTokens *float64 `json:"cache_creation_input_tokens"`
	OutputTokens             *float64 `json:"output_tokens"`
}

type RateLimits struct {
	FiveHour *RateWindow `json:"five_hour"`
	SevenDay *RateWindow `json:"seven_day"`
}

// RateWindow.ResetsAt is unix epoch seconds as a number, not an ISO-8601
// string. Measured in M0; see PRD §3.1.
type RateWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

// Decode reads a payload from r.
//
// The returned *Payload is never nil, even on error: a caller on the render
// path is expected to ignore the error and render whatever survived, which for
// unparseable input is nothing at all. The error exists so `doctor` can say
// why, and so tests can assert the difference between "absent" and "broken".
func Decode(r io.Reader) (*Payload, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxPayloadBytes))
	if err != nil {
		return &Payload{}, err
	}
	return Parse(b)
}

// Parse decodes an in-memory payload. Same contract as Decode.
func Parse(b []byte) (*Payload, error) {
	p := &Payload{raw: b}

	if len(b) == 0 {
		return p, errors.New("payload: empty input")
	}
	if err := json.Unmarshal(b, p); err != nil {
		// Partial decoding is not salvageable: encoding/json may have written
		// some fields before failing, and a half-populated payload is worse
		// than an empty one because it renders plausible nonsense.
		return &Payload{raw: b}, err
	}
	return p, nil
}

// Raw returns the bytes this payload decoded from, for KeyDiff and `capture`.
func (p *Payload) Raw() []byte { return p.raw }
