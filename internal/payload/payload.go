package payload

import (
	"encoding/json"
	"errors"
	"io"
)

const maxPayloadBytes = 4 << 20

type Payload struct {
	Model         *Model         `json:"model"`
	Workspace     *Workspace     `json:"workspace"`
	Cost          *Cost          `json:"cost"`
	ContextWindow *ContextWindow `json:"context_window"`
	RateLimits    *RateLimits    `json:"rate_limits"`
	Effort        *Effort        `json:"effort"`

	SessionID       *string `json:"session_id"`
	SessionName     *string `json:"session_name"`
	Version         *string `json:"version"`
	TranscriptPath  *string `json:"transcript_path"`
	Cwd             *string `json:"cwd"`
	Exceeds200kToks *bool   `json:"exceeds_200k_tokens"`

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

type Effort struct {
	Level *string `json:"level"`
}

type RateWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *float64 `json:"resets_at"`
}

func Decode(r io.Reader) (*Payload, error) {
	b, err := io.ReadAll(io.LimitReader(r, maxPayloadBytes))
	if err != nil {
		return &Payload{}, err
	}
	return Parse(b)
}

func Parse(b []byte) (*Payload, error) {
	p := &Payload{raw: b}

	if len(b) == 0 {
		return p, errors.New("payload: empty input")
	}
	if err := json.Unmarshal(b, p); err != nil {
		return &Payload{raw: b}, err
	}
	return p, nil
}

func (p *Payload) Raw() []byte { return p.raw }
