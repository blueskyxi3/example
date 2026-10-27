package processor

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	sdk "ai-hub-plugin-api/relay"
)

type event struct {
	unit   sdk.Unit
	object map[string]any
	fields []sdk.TextField
}
type Response struct {
	Rules        []Replacement
	Stream       bool
	Summary      sdk.Summary
	pending      []*event
	pendingBytes int
	finished     map[string]bool
}

func NewResponse(rules []Replacement, stream bool) *Response {
	return &Response{Rules: rules, Stream: stream, Summary: sdk.Summary{Action: "pass"}, finished: map[string]bool{}}
}

func (p *Response) Data(unit sdk.Unit) ([]sdk.Unit, error) {
	if len(unit.Data) > sdk.MaxUnitBytes {
		return nil, errors.New("input too large")
	}
	if len(p.Rules) == 0 {
		return []sdk.Unit{unit}, nil
	}
	data := unit.Data
	if p.Stream {
		data = sdk.EventData(data)
	}
	object, err := sdk.DecodeObject(data)
	if err != nil && !p.Stream {
		return nil, err
	}
	item := &event{unit: unit, object: object}
	if object != nil {
		item.fields = sdk.ResponseText(object)
	}
	p.pending = append(p.pending, item)
	p.pendingBytes += len(unit.Data)
	if len(p.pending) > sdk.MaxPendingUnits || p.pendingBytes > sdk.MaxPendingBytes {
		return nil, errors.New("pending window exceeded")
	}
	if !p.Stream || (sdk.IsTerminal(data) && object["candidates"] == nil) {
		return p.flush(len(p.pending))
	}
	// Only close the matching block. Flushing every block at another block's
	// stop/snapshot would leak a word split across interleaved content blocks.
	if object != nil {
		if candidates, ok := object["candidates"].([]any); ok {
			for i, value := range candidates {
				candidate, _ := value.(map[string]any)
				if candidate["finishReason"] == nil {
					continue
				}
				index := candidate["index"]
				if index == nil {
					index = i
				}
				prefix := fmt.Sprintf("gemini:%v:", index)
				for _, event := range p.pending {
					for _, field := range event.fields {
						if strings.HasPrefix(field.Key, prefix) {
							p.finished[field.Key] = true
						}
					}
				}
			}
		}
		switch object["type"] {
		case "content_block_stop":
			p.finished[fmt.Sprintf("claude:%v", object["index"])] = true
		case "response.output_text.done", "response.content_part.done":
			p.finished[fmt.Sprintf("responses:%v:%v", object["output_index"], object["content_index"])] = true
		case "response.output_item.done":
			prefix := fmt.Sprintf("responses:%v:", object["output_index"])
			for _, event := range p.pending {
				for _, field := range event.fields {
					if strings.HasPrefix(field.Key, prefix) {
						p.finished[field.Key] = true
					}
				}
			}
		}
	}
	return p.flush(p.safeCount())
}

func (p *Response) End() ([]sdk.Unit, error) { return p.flush(len(p.pending)) }

// Keep only events touching a suffix that could begin a configured word. The
// pending unit/byte bounds also cover interleaved metadata or many choices.
func (p *Response) safeCount() int {
	count := len(p.pending)
	for count > 0 {
		next := p.safePrefix(count)
		if next == count {
			return count
		}
		count = next
	}
	return 0
}

// Recheck the selected event boundary: cutting off a safe byte prefix at an
// earlier event may itself split a complete match across the held next event.
func (p *Response) safePrefix(count int) int {
	texts := map[string]string{}
	for _, item := range p.pending[:count] {
		for _, field := range item.fields {
			if field.Delta {
				texts[field.Key] += field.Text()
			}
		}
	}
	lastDelta := map[string]int{}
	for i, item := range p.pending {
		for _, field := range item.fields {
			if field.Delta {
				lastDelta[field.Key] = i
			}
		}
	}
	safeEnds := map[string]int{}
	for key, text := range texts {
		if p.finished[key] && lastDelta[key] < count {
			safeEnds[key] = len(text)
			continue
		}
		unsafe := 0
		for _, rule := range p.Rules {
			for size := 1; size < len(rule.From) && size <= len(text); size++ {
				if utf8.ValidString(rule.From[:size]) && strings.HasSuffix(text, rule.From[:size]) && size > unsafe {
					unsafe = size
				}
			}
		}
		safeEnds[key] = len(text) - unsafe
	}
	positions := map[string]int{}
	for i, item := range p.pending[:count] {
		for _, field := range item.fields {
			if field.Delta {
				positions[field.Key] += len(field.Text())
				if positions[field.Key] > safeEnds[field.Key] {
					return i
				}
			}
		}
	}
	return count
}

func (p *Response) flush(count int) ([]sdk.Unit, error) {
	if count == 0 {
		return nil, nil
	}
	items := p.pending[:count]
	groups := map[string][]sdk.TextField{}
	for _, item := range items {
		for _, field := range item.fields {
			if field.Delta && p.Stream {
				groups[field.Key] = append(groups[field.Key], field)
			} else {
				text, hits := replace(field.Text(), p.Rules)
				field.Set(text)
				p.Summary.Hits += hits
			}
		}
	}
	for _, fields := range groups {
		var text strings.Builder
		for _, field := range fields {
			text.WriteString(field.Text())
		}
		replaced, hits := replace(text.String(), p.Rules)
		p.Summary.Hits += hits
		runes := []rune(replaced)
		for i, field := range fields {
			size := min(utf8.RuneCountInString(field.Text()), len(runes))
			if i == len(fields)-1 {
				size = len(runes)
			}
			field.Set(string(runes[:size]))
			runes = runes[size:]
		}
	}
	if p.Summary.Hits > 0 {
		p.Summary.Action = "replace"
	}
	output := make([]sdk.Unit, 0, count)
	for _, item := range items {
		data := item.unit.Data
		if len(item.fields) > 0 {
			var err error
			data, err = sdk.EncodeObject(item.object, data, p.Stream)
			if err != nil {
				return nil, err
			}
		}
		if len(data) > sdk.MaxUnitBytes {
			return nil, errors.New("output too large")
		}
		output = append(output, sdk.Unit{Sequence: item.unit.Sequence, Data: data})
		p.pendingBytes -= len(item.unit.Data)
	}
	p.pending = append([]*event(nil), p.pending[count:]...)
	for key := range p.finished {
		present := false
		for _, item := range p.pending {
			for _, field := range item.fields {
				present = present || field.Key == key
			}
		}
		if !present {
			delete(p.finished, key)
		}
	}
	return output, nil
}
