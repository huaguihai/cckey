package proxy

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/huaguihai/cckey/cckey-proxy/internal/config"
)

// --- OpenAI types ---

type openAIChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message      openAIChatMessage `json:"message"`
		FinishReason string            `json:"finish_reason"`
	} `json:"choices"`
	Usage openAIUsage `json:"usage"`
}

type openAIChatMessage struct {
	Role      string           `json:"role"`
	Content   any              `json:"content"`
	Refusal   string           `json:"refusal"`
	ToolCalls []openAIToolCall `json:"tool_calls"`
}

type openAIToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIStreamChunk struct {
	ID      string       `json:"id"`
	Model   string       `json:"model"`
	Usage   *openAIUsage `json:"usage"`
	Choices []struct {
		Delta struct {
			Content   string                  `json:"content"`
			Refusal   string                  `json:"refusal"`
			ToolCalls []openAIStreamToolDelta `json:"tool_calls"`
		} `json:"delta"`
		FinishReason *string `json:"finish_reason"`
	} `json:"choices"`
}

type openAIStreamToolDelta struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolAccumulator struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// --- Handlers ---

func handleTranslatedModels(w http.ResponseWriter, profile *config.Profile) {
	models := advertisedModels(profile)
	items := make([]map[string]any, 0, len(models))
	for _, model := range models {
		items = append(items, map[string]any{
			"type":         "model",
			"id":           model,
			"display_name": model,
			"created_at":   "2026-04-01T00:00:00Z",
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items})
}

func handleTranslatedCountTokens(w http.ResponseWriter, r *http.Request, profile *config.Profile) {
	if err := requirePOST(r); err != nil {
		writeAnthropicError(w, http.StatusMethodNotAllowed, err.Error())
		return
	}

	requestBody, err := decodeBodyMap(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"input_tokens": estimateAnthropicInputTokens(requestBody),
	})
}

func advertisedModels(profile *config.Profile) []string {
	if len(profile.ModelMap) > 0 {
		models := make([]string, 0, len(profile.ModelMap))
		for model := range profile.ModelMap {
			models = append(models, model)
		}
		sort.Strings(models)
		return models
	}
	return []string{
		"claude-sonnet-4-20250514",
		"claude-opus-4-1-20250805",
	}
}

func handleTranslatedMessages(w http.ResponseWriter, r *http.Request, cfg *config.Config, profile *config.Profile) {
	if err := requirePOST(r); err != nil {
		writeAnthropicError(w, http.StatusMethodNotAllowed, err.Error())
		return
	}

	requestBody, err := decodeBodyMap(r.Body)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}

	upstreamBody, requestedModel, err := anthropicToOpenAI(profile, requestBody)
	if err != nil {
		writeAnthropicError(w, http.StatusBadRequest, err.Error())
		return
	}

	isStream, _ := upstreamBody["stream"].(bool)
	payload, err := json.Marshal(upstreamBody)
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Use request context for proper cancellation propagation
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost,
		joinURL(profile.BaseURL, "/v1/chat/completions"), bytes.NewReader(payload))
	if err != nil {
		writeAnthropicError(w, http.StatusInternalServerError, err.Error())
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if profile.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+profile.APIKey)
	}
	for key, value := range profile.Headers {
		req.Header.Set(key, value)
	}

	// For streaming, use a client without timeout (context handles cancellation)
	client := httpClient
	if isStream {
		client = &http.Client{} // no timeout — request context cancels on disconnect
	}

	resp, err := client.Do(req)
	if err != nil {
		writeAnthropicError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		writeAnthropicError(w, resp.StatusCode, readErrorMessage(body))
		return
	}

	if isStream {
		if err := relayStreamAsAnthropic(w, resp.Body, requestedModel); err != nil {
			// If we haven't started writing, we can still send an error
			return
		}
		return
	}

	var upstreamResp openAIChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&upstreamResp); err != nil {
		writeAnthropicError(w, http.StatusBadGateway, err.Error())
		return
	}
	payloadOut := openAIToAnthropicResponse(requestedModel, upstreamResp)
	writeJSON(w, http.StatusOK, payloadOut)
}

// --- Anthropic -> OpenAI translation ---

func anthropicToOpenAI(profile *config.Profile, requestBody map[string]any) (map[string]any, string, error) {
	requestedModel, _ := requestBody["model"].(string)
	result := map[string]any{
		"model": profile.ResolveModel(requestedModel),
	}

	for _, field := range []string{"max_tokens", "temperature", "top_p", "stream"} {
		if value, ok := requestBody[field]; ok {
			result[field] = value
		}
	}
	if stream, _ := result["stream"].(bool); stream && profile.Preset == "openai" {
		result["stream_options"] = map[string]any{"include_usage": true}
	}
	if value, ok := requestBody["stop_sequences"]; ok {
		result["stop"] = value
	}
	if effort := anthropicThinkingToReasoningEffort(profile, requestBody["thinking"]); effort != "" {
		result["reasoning_effort"] = effort
	}

	if tools, ok := requestBody["tools"]; ok {
		result["tools"] = anthropicToolsToOpenAI(tools)
		if profile.Preset == "openai" {
			result["parallel_tool_calls"] = false
		}
	}
	if toolChoice, ok := requestBody["tool_choice"]; ok {
		if converted := anthropicToolChoiceToOpenAI(toolChoice); converted != nil {
			result["tool_choice"] = converted
		}
	}

	systemText := extractTextBlocks(requestBody["system"])

	messages, err := anthropicMessagesToOpenAI(requestBody["messages"])
	if err != nil {
		return nil, "", err
	}
	if systemText != "" {
		messages = append([]any{map[string]any{
			"role":    "system",
			"content": systemText,
		}}, messages...)
	}
	result["messages"] = messages
	return result, requestedModel, nil
}

func anthropicThinkingToReasoningEffort(profile *config.Profile, raw any) string {
	if profile.Preset != "openai" {
		return ""
	}
	thinking, ok := raw.(map[string]any)
	if !ok {
		return ""
	}
	if stringValue(thinking["type"]) != "enabled" {
		return ""
	}

	budget := 0
	switch value := thinking["budget_tokens"].(type) {
	case float64:
		budget = int(value)
	case int:
		budget = value
	}

	switch {
	case budget >= 12000:
		return "high"
	case budget >= 4000:
		return "medium"
	case budget > 0:
		return "low"
	default:
		return "medium"
	}
}

func estimateAnthropicInputTokens(requestBody map[string]any) int {
	total := 0

	if system := requestBody["system"]; system != nil {
		total += roughTokenCount(extractTextBlocks(system))
	}
	if tools, ok := requestBody["tools"]; ok {
		total += roughTokenCount(mustJSON(tools))
	}

	messages, _ := requestBody["messages"].([]any)
	for _, rawMessage := range messages {
		message, ok := rawMessage.(map[string]any)
		if !ok {
			continue
		}
		total += 8
		total += roughTokenCount(stringValue(message["role"]))
		total += estimateContentTokens(message["content"])
	}

	if total < 1 {
		return 1
	}
	return total
}

func estimateContentTokens(raw any) int {
	switch value := raw.(type) {
	case string:
		return roughTokenCount(value)
	case []any:
		total := 0
		for _, rawBlock := range value {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				total += roughTokenCount(mustJSON(rawBlock))
				continue
			}
			total += 4
			switch stringValue(block["type"]) {
			case "text":
				total += roughTokenCount(stringValue(block["text"]))
			case "image":
				total += estimateImageTokens(block)
			case "tool_use":
				total += roughTokenCount(stringValue(block["name"]))
				total += roughTokenCount(mustJSON(block["input"]))
			case "tool_result":
				total += roughTokenCount(stringValue(block["tool_use_id"]))
				total += roughTokenCount(toolResultContent(block))
			default:
				total += roughTokenCount(mustJSON(block))
			}
		}
		return total
	default:
		return roughTokenCount(mustJSON(raw))
	}
}

func estimateImageTokens(block map[string]any) int {
	source, ok := block["source"].(map[string]any)
	if !ok {
		return 256
	}
	data := stringValue(source["data"])
	if data == "" {
		return 256
	}
	estimated := len(data) / 16
	if estimated < 256 {
		return 256
	}
	return estimated
}

func roughTokenCount(text string) int {
	if text == "" {
		return 0
	}
	count := (len(text) + 3) / 4
	if count < 1 {
		return 1
	}
	return count
}

// --- Message conversion ---

func anthropicMessagesToOpenAI(raw any) ([]any, error) {
	items, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf("messages must be an array")
	}
	var result []any
	for _, item := range items {
		message, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		switch role {
		case "user":
			result = append(result, convertUserMessage(message["content"])...)
		case "assistant":
			result = append(result, convertAssistantMessage(message["content"])...)
		default:
			if text := extractTextBlocks(message["content"]); text != "" {
				result = append(result, map[string]any{"role": role, "content": text})
			}
		}
	}
	return result, nil
}

func convertUserMessage(raw any) []any {
	if text, ok := raw.(string); ok {
		return []any{map[string]any{"role": "user", "content": text}}
	}
	blocks, ok := raw.([]any)
	if !ok {
		return nil
	}

	out := make([]any, 0, len(blocks)+1)
	contentParts := make([]any, 0, len(blocks))
	textParts := make([]string, 0, len(blocks))
	hasRichContent := false
	flushUserChunk := func() {
		if len(contentParts) == 0 && len(textParts) == 0 {
			return
		}
		if hasRichContent {
			chunk := make([]any, len(contentParts))
			copy(chunk, contentParts)
			out = append(out, map[string]any{"role": "user", "content": chunk})
		} else {
			out = append(out, map[string]any{"role": "user", "content": strings.Join(textParts, "\n")})
		}
		contentParts = contentParts[:0]
		textParts = textParts[:0]
		hasRichContent = false
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		switch block["type"] {
		case "text":
			text := stringValue(block["text"])
			textParts = append(textParts, text)
			contentParts = append(contentParts, map[string]any{
				"type": "text",
				"text": text,
			})
		case "image":
			imagePart := anthropicImageToOpenAI(block)
			if imagePart != nil {
				hasRichContent = true
				contentParts = append(contentParts, imagePart)
			}
		case "tool_result":
			flushUserChunk()
			out = append(out, map[string]any{
				"role":         "tool",
				"tool_call_id": stringValue(block["tool_use_id"]),
				"content":      toolResultContent(block),
			})
		}
	}

	flushUserChunk()
	return out
}

func convertAssistantMessage(raw any) []any {
	if text, ok := raw.(string); ok {
		return []any{map[string]any{"role": "assistant", "content": text}}
	}
	blocks, ok := raw.([]any)
	if !ok {
		return nil
	}

	out := make([]any, 0, len(blocks))
	textParts := make([]string, 0, len(blocks))
	toolCalls := make([]any, 0)
	flushAssistant := func() {
		if len(textParts) == 0 && len(toolCalls) == 0 {
			return
		}
		message := map[string]any{
			"role": "assistant",
		}
		if len(textParts) > 0 {
			message["content"] = strings.Join(textParts, "\n")
		} else {
			message["content"] = ""
		}
		if len(toolCalls) > 0 {
			message["tool_calls"] = append([]any(nil), toolCalls...)
		}
		out = append(out, message)
		textParts = textParts[:0]
		toolCalls = toolCalls[:0]
	}
	for _, rawBlock := range blocks {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			continue
		}
		switch block["type"] {
		case "text":
			if len(toolCalls) > 0 {
				flushAssistant()
			}
			textParts = append(textParts, stringValue(block["text"]))
		case "tool_use":
			toolCalls = append(toolCalls, map[string]any{
				"id":   stringValue(block["id"]),
				"type": "function",
				"function": map[string]any{
					"name":      stringValue(block["name"]),
					"arguments": mustJSON(block["input"]),
				},
			})
		}
	}

	flushAssistant()
	return out
}

func anthropicImageToOpenAI(block map[string]any) any {
	source, ok := block["source"].(map[string]any)
	if !ok {
		return nil
	}
	if stringValue(source["type"]) != "base64" {
		return nil
	}
	mediaType := stringValue(source["media_type"])
	data := stringValue(source["data"])
	if mediaType == "" || data == "" {
		return nil
	}
	return map[string]any{
		"type": "image_url",
		"image_url": map[string]any{
			"url": fmt.Sprintf("data:%s;base64,%s", mediaType, data),
		},
	}
}

func anthropicToolsToOpenAI(raw any) []any {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]any, 0, len(items))
	for _, item := range items {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        stringValue(tool["name"]),
				"description": stringValue(tool["description"]),
				"parameters":  normalizeOpenAIToolSchema(tool["input_schema"]),
			},
		})
	}
	return out
}

func normalizeOpenAIToolSchema(raw any) any {
	switch value := raw.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(value)+1)
		for key, item := range value {
			normalized[key] = normalizeOpenAIToolSchema(item)
		}
		if schemaHasObjectType(value["type"]) {
			if _, ok := normalized["properties"]; !ok {
				normalized["properties"] = map[string]any{}
			}
		}
		if schemaHasArrayType(value["type"]) {
			if _, ok := normalized["items"]; !ok {
				normalized["items"] = map[string]any{}
			}
		}
		return normalized
	case []any:
		normalized := make([]any, 0, len(value))
		for _, item := range value {
			normalized = append(normalized, normalizeOpenAIToolSchema(item))
		}
		return normalized
	default:
		return raw
	}
}

func schemaHasArrayType(raw any) bool {
	switch value := raw.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "array")
	case []any:
		for _, item := range value {
			if schemaHasArrayType(item) {
				return true
			}
		}
	}
	return false
}

func schemaHasObjectType(raw any) bool {
	switch value := raw.(type) {
	case string:
		return strings.EqualFold(strings.TrimSpace(value), "object")
	case []any:
		for _, item := range value {
			if schemaHasObjectType(item) {
				return true
			}
		}
	}
	return false
}

func anthropicToolChoiceToOpenAI(raw any) any {
	choice, ok := raw.(map[string]any)
	if !ok {
		return nil
	}
	switch stringValue(choice["type"]) {
	case "auto":
		return "auto"
	case "any":
		return "required"
	case "tool":
		return map[string]any{
			"type": "function",
			"function": map[string]any{
				"name": stringValue(choice["name"]),
			},
		}
	default:
		return nil
	}
}

// --- OpenAI -> Anthropic response translation ---

func openAIToAnthropicResponse(requestedModel string, upstreamResp openAIChatResponse) map[string]any {
	response := map[string]any{
		"id":            upstreamResp.ID,
		"type":          "message",
		"role":          "assistant",
		"model":         requestedModel,
		"content":       []any{},
		"stop_reason":   "end_turn",
		"stop_sequence": nil,
		"usage": map[string]any{
			"input_tokens":  upstreamResp.Usage.PromptTokens,
			"output_tokens": upstreamResp.Usage.CompletionTokens,
		},
	}
	if len(upstreamResp.Choices) == 0 {
		return response
	}
	choice := upstreamResp.Choices[0]
	response["content"] = openAIMessageToAnthropicBlocks(choice.Message)
	response["stop_reason"] = finishReasonToStopReason(choice.FinishReason, len(choice.Message.ToolCalls) > 0)
	return response
}

func openAIMessageToAnthropicBlocks(message openAIChatMessage) []any {
	blocks := make([]any, 0)
	switch content := message.Content.(type) {
	case string:
		if content != "" {
			blocks = append(blocks, map[string]any{
				"type": "text",
				"text": content,
			})
		}
	case []any:
		for _, rawPart := range content {
			part, ok := rawPart.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := openAIContentPartText(part); ok {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": text,
				})
			}
		}
	}
	if len(blocks) == 0 && message.Refusal != "" {
		blocks = append(blocks, map[string]any{
			"type": "text",
			"text": message.Refusal,
		})
	}
	for _, toolCall := range message.ToolCalls {
		input := map[string]any{}
		if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
			input["_raw"] = toolCall.Function.Arguments
		}
		blocks = append(blocks, map[string]any{
			"type":  "tool_use",
			"id":    toolCall.ID,
			"name":  toolCall.Function.Name,
			"input": input,
		})
	}
	return blocks
}

func openAIContentPartText(part map[string]any) (string, bool) {
	text := stringValue(part["text"])
	if text != "" {
		switch stringValue(part["type"]) {
		case "", "text", "output_text":
			return text, true
		}
	}

	refusal := stringValue(part["refusal"])
	if refusal == "" {
		return "", false
	}
	switch stringValue(part["type"]) {
	case "refusal":
		return refusal, true
	default:
		return "", false
	}
}

// --- Stream relay ---

func relayStreamAsAnthropic(w http.ResponseWriter, body io.Reader, requestedModel string) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming is not supported by this response writer")
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	translator := newStreamTranslator(w, flusher, requestedModel)
	reader := bufio.NewReader(body)
	var dataLines []string
	headerWritten := false

	processEvent := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		eventData := strings.Join(dataLines, "\n")
		dataLines = nil
		if eventData == "[DONE]" {
			return translator.finish()
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(eventData), &payload); err == nil {
			if digString(payload, "error", "message") != "" || digString(payload, "message") != "" {
				return fmt.Errorf("%s", readErrorMessage([]byte(eventData)))
			}
			if text, ok := payload["error"].(string); ok && strings.TrimSpace(text) != "" {
				return fmt.Errorf("%s", text)
			}
		}

		var chunk openAIStreamChunk
		if err := json.Unmarshal([]byte(eventData), &chunk); err != nil {
			return err
		}
		if err := translator.start(); err != nil {
			return err
		}
		headerWritten = true
		if chunk.Usage != nil {
			translator.setUsage(*chunk.Usage)
		}
		for _, choice := range chunk.Choices {
			if choice.Delta.Content != "" {
				if err := translator.text(choice.Delta.Content); err != nil {
					return err
				}
			}
			if choice.Delta.Refusal != "" {
				if err := translator.text(choice.Delta.Refusal); err != nil {
					return err
				}
			}
			if len(choice.Delta.ToolCalls) > 0 {
				if err := translator.addToolDelta(choice.Delta.ToolCalls); err != nil {
					return err
				}
			}
			if choice.FinishReason != nil {
				translator.finishReason = *choice.FinishReason
			}
		}
		return nil
	}

	for {
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF {
				break
			}
			if !headerWritten {
				writeAnthropicError(w, http.StatusBadGateway, err.Error())
			}
			return err
		}

		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(line[5:]))
		} else if line == "" {
			if err := processEvent(); err != nil {
				if !headerWritten {
					writeAnthropicError(w, http.StatusBadGateway, err.Error())
				}
				return err
			}
		}

		if err == io.EOF {
			break
		}
	}

	if err := processEvent(); err != nil {
		return err
	}
	return translator.finish()
}

// --- Helpers ---

func extractTextBlocks(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		parts := make([]string, 0, len(value))
		for _, rawBlock := range value {
			block, ok := rawBlock.(map[string]any)
			if !ok {
				continue
			}
			if stringValue(block["type"]) == "text" {
				parts = append(parts, stringValue(block["text"]))
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}

func stringifyBlockContent(raw any) string {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		if isTextOnlyBlockList(value) {
			return extractTextBlocks(value)
		}
		return mustJSON(raw)
	default:
		return mustJSON(raw)
	}
}

func toolResultContent(block map[string]any) string {
	content := block["content"]
	if isError, _ := block["is_error"].(bool); isError {
		payload := map[string]any{
			"is_error": true,
			"content":  normalizeToolResultContent(content),
		}
		return mustJSON(payload)
	}
	return stringifyBlockContent(content)
}

func normalizeToolResultContent(raw any) any {
	switch value := raw.(type) {
	case string:
		return value
	case []any:
		if isTextOnlyBlockList(value) {
			return extractTextBlocks(value)
		}
		return value
	default:
		return raw
	}
}

func isTextOnlyBlockList(items []any) bool {
	if len(items) == 0 {
		return false
	}
	for _, rawBlock := range items {
		block, ok := rawBlock.(map[string]any)
		if !ok {
			return false
		}
		if stringValue(block["type"]) != "text" {
			return false
		}
	}
	return true
}

func stringValue(raw any) string {
	if text, ok := raw.(string); ok {
		return text
	}
	return ""
}

func mustJSON(raw any) string {
	data, err := json.Marshal(raw)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func finishReasonToStopReason(finishReason string, hasToolCalls bool) string {
	switch finishReason {
	case "tool_calls", "function_call":
		return "tool_use"
	case "length":
		return "max_tokens"
	case "stop", "":
		if hasToolCalls {
			return "tool_use"
		}
		return "end_turn"
	default:
		return "end_turn"
	}
}
