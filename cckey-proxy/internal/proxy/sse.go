package proxy

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"time"
)

type streamTranslator struct {
	writer       http.ResponseWriter
	flusher      http.Flusher
	model        string
	messageID    string
	started      bool
	textOpened   bool
	textIndex    int
	nextIndex    int
	finished     bool
	finishReason string
	inputTokens  int
	outputTokens int
	tools        map[int]*streamToolState
	activeTool   *int
}

type streamToolState struct {
	toolAccumulator
	blockIndex     int
	started        bool
	stopped        bool
	emittedArgSize int
}

func newStreamTranslator(writer http.ResponseWriter, flusher http.Flusher, model string) *streamTranslator {
	return &streamTranslator{
		writer:    writer,
		flusher:   flusher,
		model:     model,
		messageID: fmt.Sprintf("msg_cckey_%d", time.Now().UnixNano()),
		tools:     map[int]*streamToolState{},
	}
}

func (t *streamTranslator) start() error {
	if t.started {
		return nil
	}
	t.started = true
	return writeSSE(t.writer, t.flusher, "message_start", map[string]any{
		"type": "message_start",
		"message": map[string]any{
			"id":            t.messageID,
			"type":          "message",
			"role":          "assistant",
			"content":       []any{},
			"model":         t.model,
			"stop_reason":   nil,
			"stop_sequence": nil,
			"usage": map[string]any{
				"input_tokens":  0,
				"output_tokens": 0,
			},
		},
	})
}

func (t *streamTranslator) text(fragment string) error {
	if err := t.start(); err != nil {
		return err
	}
	if err := t.closeReadyToolBlocks(); err != nil {
		return err
	}
	if !t.textOpened {
		t.textOpened = true
		t.textIndex = t.nextIndex
		t.nextIndex++
		if err := writeSSE(t.writer, t.flusher, "content_block_start", map[string]any{
			"type":  "content_block_start",
			"index": t.textIndex,
			"content_block": map[string]any{
				"type": "text",
				"text": "",
			},
		}); err != nil {
			return err
		}
	}
	return writeSSE(t.writer, t.flusher, "content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": t.textIndex,
		"delta": map[string]any{
			"type": "text_delta",
			"text": fragment,
		},
	})
}

func (t *streamTranslator) addToolDelta(deltas []openAIStreamToolDelta) error {
	if err := t.start(); err != nil {
		return err
	}
	for _, delta := range deltas {
		acc, ok := t.tools[delta.Index]
		if !ok {
			acc = &streamToolState{}
			t.tools[delta.Index] = acc
		}
		if delta.ID != "" {
			acc.ID = delta.ID
		}
		if delta.Function.Name != "" {
			acc.Name = delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			acc.Arguments.WriteString(delta.Function.Arguments)
		}
		if !acc.started && acc.ID != "" && acc.Name != "" {
			if t.activeTool == nil {
				if err := t.closeTextBlock(); err != nil {
					return err
				}
				acc.blockIndex = t.nextIndex
				t.nextIndex++
				if err := writeSSE(t.writer, t.flusher, "content_block_start", map[string]any{
					"type":  "content_block_start",
					"index": acc.blockIndex,
					"content_block": map[string]any{
						"type":  "tool_use",
						"id":    acc.ID,
						"name":  acc.Name,
						"input": map[string]any{},
					},
				}); err != nil {
					return err
				}
				acc.started = true
				index := delta.Index
				t.activeTool = &index
			}
		}
		// Stream tool arguments as they arrive
		if acc.started && t.activeTool != nil && *t.activeTool == delta.Index && !acc.stopped {
			if err := t.emitPendingToolArguments(acc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (t *streamTranslator) setUsage(usage openAIUsage) {
	t.inputTokens = usage.PromptTokens
	t.outputTokens = usage.CompletionTokens
}

func (t *streamTranslator) closeTextBlock() error {
	if !t.textOpened {
		return nil
	}
	if err := writeSSE(t.writer, t.flusher, "content_block_stop", map[string]any{
		"type":  "content_block_stop",
		"index": t.textIndex,
	}); err != nil {
		return err
	}
	t.textOpened = false
	return nil
}

func (t *streamTranslator) closeToolBlocks() error {
	indices := t.sortedToolIndices()
	for _, index := range indices {
		acc := t.tools[index]
		if !acc.started {
			if acc.ID == "" {
				acc.ID = fmt.Sprintf("toolu_cckey_%d", index)
			}
			if err := t.closeTextBlock(); err != nil {
				return err
			}
			acc.blockIndex = t.nextIndex
			t.nextIndex++
			if err := writeSSE(t.writer, t.flusher, "content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": acc.blockIndex,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    acc.ID,
					"name":  acc.Name,
					"input": map[string]any{},
				},
			}); err != nil {
				return err
			}
			acc.started = true
		}
		if acc.started && !acc.stopped {
			if err := t.emitPendingToolArguments(acc); err != nil {
				return err
			}
			if err := writeSSE(t.writer, t.flusher, "content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": acc.blockIndex,
			}); err != nil {
				return err
			}
			acc.stopped = true
		}
	}
	t.activeTool = nil
	return nil
}

func (t *streamTranslator) closeReadyToolBlocks() error {
	indices := t.sortedToolIndices()
	for _, index := range indices {
		acc := t.tools[index]
		if !acc.started {
			if acc.ID == "" || acc.Name == "" {
				continue
			}
			if err := t.closeTextBlock(); err != nil {
				return err
			}
			acc.blockIndex = t.nextIndex
			t.nextIndex++
			if err := writeSSE(t.writer, t.flusher, "content_block_start", map[string]any{
				"type":  "content_block_start",
				"index": acc.blockIndex,
				"content_block": map[string]any{
					"type":  "tool_use",
					"id":    acc.ID,
					"name":  acc.Name,
					"input": map[string]any{},
				},
			}); err != nil {
				return err
			}
			acc.started = true
		}
		if acc.started && !acc.stopped {
			if err := t.emitPendingToolArguments(acc); err != nil {
				return err
			}
			if err := writeSSE(t.writer, t.flusher, "content_block_stop", map[string]any{
				"type":  "content_block_stop",
				"index": acc.blockIndex,
			}); err != nil {
				return err
			}
			acc.stopped = true
		}
	}
	t.activeTool = nil
	return nil
}

func (t *streamTranslator) finish() error {
	if t.finished {
		return nil
	}
	if err := t.start(); err != nil {
		return err
	}
	t.finished = true

	if err := t.closeTextBlock(); err != nil {
		return err
	}
	if err := t.closeToolBlocks(); err != nil {
		return err
	}

	stopReason := finishReasonToStopReason(t.finishReason, len(t.tools) > 0)
	if err := writeSSE(t.writer, t.flusher, "message_delta", map[string]any{
		"type": "message_delta",
		"delta": map[string]any{
			"stop_reason":   stopReason,
			"stop_sequence": nil,
		},
		"usage": map[string]any{
			"input_tokens":  t.inputTokens,
			"output_tokens": t.outputTokens,
		},
	}); err != nil {
		return err
	}

	return writeSSE(t.writer, t.flusher, "message_stop", map[string]any{
		"type": "message_stop",
	})
}

func (t *streamTranslator) emitPendingToolArguments(acc *streamToolState) error {
	full := acc.Arguments.String()
	if acc.emittedArgSize >= len(full) {
		return nil
	}
	pending := full[acc.emittedArgSize:]
	if pending == "" {
		return nil
	}
	if err := writeSSE(t.writer, t.flusher, "content_block_delta", map[string]any{
		"type":  "content_block_delta",
		"index": acc.blockIndex,
		"delta": map[string]any{
			"type":         "input_json_delta",
			"partial_json": pending,
		},
	}); err != nil {
		return err
	}
	acc.emittedArgSize = len(full)
	return nil
}

func (t *streamTranslator) sortedToolIndices() []int {
	indices := make([]int, 0, len(t.tools))
	for index := range t.tools {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	return indices
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
