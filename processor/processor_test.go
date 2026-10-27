package processor_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	sdk "ai-hub-plugin-api/relay"

	"github.com/stretchr/testify/require"

	"ai-hub-example/processor"
)

func chunk(index int, text string) []byte {
	data, _ := json.Marshal(map[string]any{"id": "test", "model": "m", "choices": []any{map[string]any{"index": index, "delta": map[string]any{"content": text}, "finish_reason": nil}}})
	return []byte("data: " + string(data) + "\n\n")
}

func runChunks(t *testing.T, rules []processor.Replacement, chunks [][]byte) ([]sdk.Unit, *processor.Response) {
	t.Helper()
	p := processor.NewResponse(rules, true)
	outputs := []sdk.Unit{}
	for i, data := range chunks {
		units, err := p.Data(sdk.Unit{Sequence: i + 1, Data: data})
		require.NoError(t, err)
		outputs = append(outputs, units...)
	}
	units, err := p.End()
	require.NoError(t, err)
	outputs = append(outputs, units...)
	require.Len(t, outputs, len(chunks))
	for i, unit := range outputs {
		require.Equal(t, i+1, unit.Sequence)
		require.NoError(t, sdk.ValidateOutput(chunks[i], unit.Data, true))
	}
	return outputs, p
}

func textOutput(units []sdk.Unit, key string) string {
	var text strings.Builder
	for _, unit := range units {
		object, err := sdk.DecodeObject(sdk.EventData(unit.Data))
		if err != nil {
			continue
		}
		for _, field := range sdk.ResponseText(object) {
			if field.Key == key && field.Delta {
				text.WriteString(field.Text())
			}
		}
	}
	return text.String()
}

func TestCrossEventReplacementAtEverySplit(t *testing.T) {
	rules := []processor.Replacement{{From: "abcd", To: "[x]"}}
	input := "!abcdab!abcd!"
	for a := 1; a < len(input); a++ {
		for b := a + 1; b < len(input); b++ {
			t.Run(fmt.Sprintf("%d-%d", a, b), func(t *testing.T) {
				units, _ := runChunks(t, rules, [][]byte{chunk(0, input[:a]), chunk(0, input[a:b]), chunk(0, input[b:]), []byte("data: [DONE]\n\n")})
				require.Equal(t, "![x]ab![x]!", textOutput(units, "choice:0:content"))
			})
		}
	}
}

func TestChoicesAndUnicodeAreIsolated(t *testing.T) {
	rules := []processor.Replacement{{From: "保密信息", To: "[隐藏]"}}
	units, p := runChunks(t, rules, [][]byte{chunk(0, "保密"), chunk(1, "信息"), chunk(0, "信息!"), []byte("data: [DONE]\n\n")})
	require.Equal(t, "[隐藏]!", textOutput(units, "choice:0:content"))
	require.Equal(t, "信息", textOutput(units, "choice:1:content"))
	require.Equal(t, 1, p.Summary.Hits)
}

func TestFourProtocolsPreserveUsageAndSnapshots(t *testing.T) {
	rules := []processor.Replacement{{From: "secret", To: "[safe]"}}
	for _, fixture := range []struct {
		name   string
		chunks [][]byte
		key    string
	}{
		{"claude", [][]byte{[]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"sec\"}}\n\n"), []byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ret\"}}\n\n"), []byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")}, "claude:0"},
		{"responses", [][]byte{[]byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"sec\"}\n\n"), []byte("data: {\"type\":\"response.output_text.delta\",\"output_index\":0,\"content_index\":0,\"delta\":\"ret\"}\n\n"), []byte("data: {\"type\":\"response.completed\",\"response\":{\"output\":[{\"content\":[{\"type\":\"output_text\",\"text\":\"secret\"}]}],\"usage\":{\"total_tokens\":17}}}\n\n")}, "responses:0:0"},
		{"gemini", [][]byte{[]byte("data: {\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"text\":\"sec\"}]}}]}\n\n"), []byte("data: {\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"text\":\"ret\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"totalTokenCount\":17}}\n\n")}, "gemini:0:0:<nil>"},
		{"openai", [][]byte{chunk(0, "sec"), chunk(0, "ret"), []byte("data: {\"choices\":[],\"usage\":{\"total_tokens\":17}}\n\n"), []byte("data: [DONE]\n\n")}, "choice:0:content"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			units, _ := runChunks(t, rules, fixture.chunks)
			require.Equal(t, "[safe]", textOutput(units, fixture.key))
			if fixture.name == "responses" {
				require.Contains(t, string(units[len(units)-1].Data), `"text":"[safe]"`)
				require.Contains(t, string(units[len(units)-1].Data), `"total_tokens":17`)
			}
		})
	}
}

func TestRequestRejectAndReplace(t *testing.T) {
	rules := processor.Rules{RequestBlocked: []string{"BLOCK"}, RequestReplacements: []processor.Replacement{{From: "private", To: "safe"}}}
	require.NoError(t, rules.Validate())
	for _, body := range []string{`{"messages":[{"role":"user","content":"BLOCK"}]}`, `{"input":"BLOCK"}`, `{"contents":[{"parts":[{"text":"BLOCK"}]}]}`} {
		_, summary, failure := rules.Request([]byte(body))
		require.Equal(t, "reject", summary.Action)
		require.Equal(t, "request_rejected", failure.Code)
		require.Equal(t, "block content", failure.Message)
	}
	output, summary, failure := rules.Request([]byte(`{"model":"m","messages":[{"content":"private","tool_calls":[{"function":{"arguments":"private"}}]}]}`))
	require.Nil(t, failure)
	require.Equal(t, 1, summary.Hits)
	require.Contains(t, string(output), `"content":"safe"`)
	require.Contains(t, string(output), `"arguments":"private"`)
}

func TestDefaultRulesAreByteExactAndPendingBounded(t *testing.T) {
	data := chunk(0, "normal")
	p := processor.NewResponse(nil, true)
	units, err := p.Data(sdk.Unit{Sequence: 1, Data: data})
	require.NoError(t, err)
	require.Equal(t, data, units[0].Data)
	p = processor.NewResponse([]processor.Replacement{{From: "secret", To: "x"}}, true)
	_, err = p.Data(sdk.Unit{Sequence: 1, Data: chunk(0, "sec")})
	require.NoError(t, err)
	for i := 0; i < sdk.MaxPendingUnits; i++ {
		_, err = p.Data(sdk.Unit{Sequence: i + 2, Data: []byte(": heartbeat\n\n")})
		if err != nil {
			break
		}
	}
	require.Error(t, err)
}

func TestInterleavedBlockStopCannotFlushOtherBlockTail(t *testing.T) {
	for _, protocol := range []string{"claude", "responses", "gemini"} {
		t.Run(protocol, func(t *testing.T) {
			var a, b, stop, c string
			key := "claude:0"
			if protocol == "claude" {
				a = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"sec"}}`
				b = `{"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"hello"}}`
				stop = `{"type":"content_block_stop","index":1}`
				c = `{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"ret"}}`
			} else if protocol == "gemini" {
				key = "gemini:0:0:<nil>"
				a = `{"candidates":[{"index":0,"content":{"parts":[{"text":"sec"}]}}]}`
				b = `{"candidates":[{"index":1,"content":{"parts":[{"text":"hello"}]}}]}`
				stop = `{"candidates":[{"index":1,"finishReason":"STOP"}]}`
				c = `{"candidates":[{"index":0,"content":{"parts":[{"text":"ret"}]},"finishReason":"STOP"}]}`
			} else {
				key = "responses:0:0"
				a = `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"sec"}`
				b = `{"type":"response.output_text.delta","output_index":1,"content_index":0,"delta":"hello"}`
				stop = `{"type":"response.output_text.done","output_index":1,"content_index":0,"text":"hello"}`
				c = `{"type":"response.output_text.delta","output_index":0,"content_index":0,"delta":"ret"}`
			}
			chunks := [][]byte{}
			for _, value := range []string{a, b, stop, c} {
				chunks = append(chunks, []byte("data: "+value+"\n\n"))
			}
			outputs, _ := runChunks(t, []processor.Replacement{{From: "secret", To: "[safe]"}}, chunks)
			require.Equal(t, "[safe]", textOutput(outputs, key))
		})
	}
}
