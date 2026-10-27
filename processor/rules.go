package processor

import (
	"errors"
	"strings"
	"unicode/utf8"

	sdk "ai-hub-plugin-api/relay"
)

type Replacement struct {
	From string `mapstructure:"from"`
	To   string `mapstructure:"to"`
}
type Rules struct {
	RequestBlocked       []string      `mapstructure:"request_blocked"`
	RequestReplacements  []Replacement `mapstructure:"request_replacements"`
	ResponseReplacements []Replacement `mapstructure:"response_replacements"`
}

func (r Rules) Validate() error {
	for _, rules := range [][]Replacement{r.RequestReplacements, r.ResponseReplacements} {
		if len(rules) > 64 {
			return errors.New("最多支持 64 条替换规则")
		}
		seen := map[string]bool{}
		for _, rule := range rules {
			if rule.From == "" || utf8.RuneCountInString(rule.From) > 128 || len(rule.To) > 1024 || !utf8.ValidString(rule.From+rule.To) || seen[rule.From] {
				return errors.New("替换规则无效：匹配词须为 1–128 字符且不重复，替换值最多 1024 字节")
			}
			seen[rule.From] = true
		}
	}
	if len(r.RequestBlocked) > 64 {
		return errors.New("最多支持 64 个拦截关键词")
	}
	for _, word := range r.RequestBlocked {
		if word == "" || len(word) > 1024 {
			return errors.New("拦截关键词无效")
		}
	}
	return nil
}

// replace applies the first configured match at each position. Replacement text
// is never reprocessed, making batch and streaming behavior identical.
func replace(text string, rules []Replacement) (string, int) {
	var output strings.Builder
	hits := 0
	for len(text) > 0 {
		matched := false
		for _, rule := range rules {
			if strings.HasPrefix(text, rule.From) {
				output.WriteString(rule.To)
				text = text[len(rule.From):]
				hits++
				matched = true
				break
			}
		}
		if !matched {
			_, size := utf8.DecodeRuneInString(text)
			output.WriteString(text[:size])
			text = text[size:]
		}
	}
	return output.String(), hits
}

func requestText(object map[string]any) []sdk.TextField {
	var result []sdk.TextField
	add := func(obj map[string]any, key string) {
		if _, ok := obj[key].(string); ok {
			result = append(result, sdk.TextField{Object: obj, Name: key})
		}
	}
	var content func(any)
	content = func(value any) {
		items, _ := value.([]any)
		for _, item := range items {
			obj, _ := item.(map[string]any)
			switch obj["type"] {
			case "text", "input_text", "output_text":
				add(obj, "text")
			}
			add(obj, "content")
			content(obj["content"])
			if parts, ok := obj["parts"].([]any); ok {
				for _, part := range parts {
					p, _ := part.(map[string]any)
					add(p, "text")
				}
			}
		}
	}
	for _, key := range []string{"prompt", "input", "instructions", "system"} {
		add(object, key)
		content(object[key])
	}
	content(object["messages"])
	content(object["contents"])
	if system, ok := object["systemInstruction"].(map[string]any); ok {
		content([]any{system})
	}
	return result
}

func (r Rules) Request(body []byte) ([]byte, sdk.Summary, *sdk.Failure) {
	object, err := sdk.DecodeObject(body)
	if err != nil {
		return nil, sdk.Summary{}, &sdk.Failure{Code: "invalid_request"}
	}
	fields := requestText(object)
	for _, field := range fields {
		for _, blocked := range r.RequestBlocked {
			if strings.Contains(field.Text(), blocked) {
				return nil, sdk.Summary{Action: "reject", Hits: 1}, &sdk.Failure{Code: "request_rejected", Message: "block content"}
			}
		}
	}
	summary := sdk.Summary{Action: "pass"}
	for _, field := range fields {
		text, hits := replace(field.Text(), r.RequestReplacements)
		field.Set(text)
		summary.Hits += hits
	}
	if summary.Hits == 0 {
		return body, summary, nil
	}
	summary.Action = "replace"
	output, err := sdk.EncodeObject(object, body, false)
	if err != nil {
		return nil, summary, &sdk.Failure{Code: "processing_failed"}
	}
	return output, summary, nil
}
