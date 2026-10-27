package controller

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	sdk "ai-hub-plugin-api/relay"

	"github.com/gin-gonic/gin"

	"ai-hub-example/model"
	"ai-hub-example/processor"
)

type execution struct {
	record          model.Record
	start, lastSave time.Time
}

func (s *Service) begin(c *gin.Context, meta sdk.Metadata, point string) (*execution, error) {
	if meta.UserID <= 0 || meta.RequestID == "" || len(meta.RequestID) > 128 || meta.Attempt < 1 {
		return nil, errors.New("invalid metadata")
	}
	now := time.Now()
	e := &execution{start: now, lastSave: now, record: model.Record{RequestID: meta.RequestID, UserID: meta.UserID, TokenID: meta.TokenID, Point: point, Attempt: meta.Attempt, ChannelID: meta.ChannelID, Protocol: meta.Protocol, Model: meta.Model, Stream: meta.Stream, Debug: meta.Debug, Status: "running", Action: "pass", CreatedAt: now.Unix(), UpdatedAt: now.Unix()}}
	if err := s.Store.Start(c.Request.Context(), &e.record); err != nil {
		return nil, err
	}
	return e, nil
}

func (e *execution) capture(data []byte, input bool) {
	if input {
		e.record.InputEvents++
		e.record.InputBytes += int64(len(data))
	} else {
		e.record.OutputEvents++
		e.record.OutputBytes += int64(len(data))
	}
	if e.record.Stream && e.record.Point == sdk.ResponsePoint {
		data = sdk.EventData(data)
		if sdk.IsTerminal(data) {
			e.record.Terminal = true
		}
		if !sdk.HasContent(data) {
			return
		}
	}
	if !e.record.Debug {
		return
	}
	text, truncated := sdk.Snapshot(data)
	if input {
		e.record.Input = text
		e.record.InputTruncated = truncated
	} else {
		e.record.Output = text
		e.record.OutputTruncated = truncated
	}
}

func (s *Service) save(c *gin.Context, e *execution, status, code string) error {
	e.record.Status = status
	e.record.ErrorCode = code
	e.record.UpdatedAt = time.Now().Unix()
	e.record.DurationMs = time.Since(e.start).Milliseconds()
	e.lastSave = time.Now()
	if err := s.Store.Save(c.Request.Context(), &e.record); err != nil {
		slog.Error("execution record persistence failed", "request_id", e.record.RequestID, "error", err)
		return err
	}
	return nil
}

func (s *Service) Request(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 48<<20)
	var input sdk.Request
	if c.ShouldBindJSON(&input) != nil {
		c.JSON(400, sdk.RequestResult{Error: &sdk.Failure{Code: "invalid_request"}})
		return
	}
	e, err := s.begin(c, input.Metadata, sdk.RequestPoint)
	if err != nil {
		c.JSON(500, sdk.RequestResult{Error: &sdk.Failure{Code: "record_unavailable"}})
		return
	}
	defer func() {
		if value := recover(); value != nil {
			slog.Error("request processing panic", "request_id", e.record.RequestID, "stack", string(debug.Stack()))
			_ = s.save(c, e, "error", "processing_panic")
			c.JSON(500, sdk.RequestResult{Error: &sdk.Failure{Code: "processing_panic"}})
		}
	}()
	e.capture(input.Body, true)
	body, summary, failure := s.Rules.Request(input.Body)
	e.record.Action = summary.Action
	e.record.Hits = summary.Hits
	status, code := "completed", ""
	if failure != nil {
		status, code = "error", failure.Code
	} else {
		e.capture(body, false)
	}
	if err = s.save(c, e, status, code); err != nil {
		failure = &sdk.Failure{Code: "record_unavailable"}
		body = nil
	}
	c.JSON(200, sdk.RequestResult{Body: body, Summary: summary, Error: failure})
}

func (s *Service) Response(c *gin.Context) {
	decoder := sdk.NewDecoder(c.Request.Body)
	var frame sdk.Frame
	if decoder.Decode(&frame) != nil || frame.Type != "begin" || frame.Metadata == nil {
		c.Status(400)
		return
	}
	e, err := s.begin(c, *frame.Metadata, sdk.ResponsePoint)
	if err != nil {
		c.Status(500)
		return
	}
	c.Header("Content-Type", "application/x-ndjson")
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no")
	encoder := json.NewEncoder(c.Writer)
	emit := func(frame sdk.Frame) error { err := encoder.Encode(frame); c.Writer.Flush(); return err }
	finished := false
	defer func() {
		if value := recover(); value != nil {
			slog.Error("response processing panic", "request_id", e.record.RequestID, "stack", string(debug.Stack()))
			_ = s.save(c, e, "error", "processing_panic")
			_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "processing_panic"}})
			return
		}
		if !finished {
			status := "error"
			if c.Request.Context().Err() != nil {
				status = "canceled"
			}
			_ = s.save(c, e, status, "stream_interrupted")
		}
	}()
	if emit(sdk.Frame{Type: "ready"}) != nil {
		return
	}
	processor := processor.NewResponse(s.Rules.ResponseReplacements, frame.Metadata.Stream)
	sequence := 0
	for {
		var frame sdk.Frame
		if err = decoder.Decode(&frame); err != nil {
			if err != io.EOF {
				slog.Error("invalid relay stream", "request_id", e.record.RequestID)
			}
			return
		}
		var output []sdk.Unit
		switch frame.Type {
		case "data":
			if frame.Unit == nil || frame.Unit.Sequence != sequence+1 {
				_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "protocol_error"}})
				return
			}
			sequence++
			e.capture(frame.Unit.Data, true)
			output, err = processor.Data(*frame.Unit)
		case "end":
			output, err = processor.End()
		default:
			_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "protocol_error"}})
			return
		}
		if err != nil {
			_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "processing_failed"}})
			return
		}
		for _, unit := range output {
			e.capture(unit.Data, false)
		}
		e.record.Action = processor.Summary.Action
		e.record.Hits = processor.Summary.Hits
		typeName := "ack"
		if frame.Type == "end" {
			if sequence == 0 || (e.record.Stream && !e.record.Terminal) {
				_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "stream_incomplete"}})
				return
			}
			typeName = "end"
			if s.save(c, e, "completed", "") != nil {
				_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "record_unavailable"}})
				return
			}
		} else if time.Since(e.lastSave) >= time.Second {
			if s.save(c, e, "running", "") != nil {
				_ = emit(sdk.Frame{Type: "error", Error: &sdk.Failure{Code: "record_unavailable"}})
				return
			}
		}
		if emit(sdk.Frame{Type: typeName, Outputs: output, Summary: processor.Summary}) != nil {
			return
		}
		finished = frame.Type == "end"
		if finished {
			return
		}
	}
}
