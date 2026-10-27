package router_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	sdk "ai-hub-plugin-api/relay"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"ai-hub-example/controller"
	"ai-hub-example/hostclient"
	"ai-hub-example/middleware"
	"ai-hub-example/model"
	"ai-hub-example/processor"
	"ai-hub-example/router"
)

const secret = "12345678901234567890123456789012"

func fixture(t *testing.T) (*controller.Service, *httptest.Server, *atomic.Int64) {
	t.Helper()
	var role atomic.Int64
	role.Store(1)
	host := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/user/self" || r.Header.Get("X-AIHub-Plugin-ID") != "example" {
			w.WriteHeader(403)
			return
		}
		user, err := strconv.Atoi(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer delegate-"))
		if err != nil {
			w.WriteHeader(403)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "data": map[string]any{"id": user, "role": role.Load(), "status": 1}})
	}))
	t.Cleanup(host.Close)
	v := viper.New()
	v.Set("database.mode", "independent")
	v.Set("database.driver", "sqlite")
	v.Set("database.sqlite_path", filepath.Join(t.TempDir(), "example.db"))
	v.Set("database.sqlite_busy_timeout", 3000)
	store, err := model.Open(v)
	require.NoError(t, err)
	t.Cleanup(func() { db, _ := store.DB.DB(); _ = db.Close() })
	client, err := hostclient.New(host.URL, "")
	require.NoError(t, err)
	s := &controller.Service{Store: store, Host: client, Rules: processor.Rules{RequestBlocked: []string{"BLOCK"}, RequestReplacements: []processor.Replacement{{From: "private", To: "safe"}}, ResponseReplacements: []processor.Replacement{{From: "secret", To: "redacted"}}}}
	manifest, err := os.ReadFile("../plugin.json")
	require.NoError(t, err)
	server := httptest.NewServer(router.New(s, secret, fstest.MapFS{"assets/plugin.js": {Data: []byte("export function Page() {}")}}, manifest))
	t.Cleanup(server.Close)
	return s, server, &role
}

func signedIdentity(req *http.Request, user int, expires int64, plugin string) string {
	data, _ := json.Marshal(middleware.Identity{UserID: user, PluginID: plugin, Method: req.Method, URI: req.URL.RequestURI(), RequestID: "page-request", ExpiresAt: expires})
	body := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func pageRequest(t *testing.T, server *httptest.Server, path string, user int) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
	require.NoError(t, err)
	req.Header.Set(middleware.IdentityHeader, signedIdentity(req, user, time.Now().Add(30*time.Second).Unix(), "example"))
	req.Header.Set(hostclient.AuthorizationHeader, fmt.Sprintf("delegate-%d", user))
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

func TestRecordAuthorizationUsesCurrentHostRole(t *testing.T) {
	s, server, role := fixture(t)
	require.NoError(t, s.Store.DB.Create(&model.Record{UserID: 7, RequestID: "mine", Point: sdk.RequestPoint, Status: "completed"}).Error)
	other := model.Record{UserID: 8, RequestID: "other", Point: sdk.ResponsePoint, Status: "completed"}
	require.NoError(t, s.Store.DB.Create(&other).Error)
	resp := pageRequest(t, server, "/api/v1/records?user_id=8", 7)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode)
	require.Contains(t, string(data), "mine")
	require.NotContains(t, string(data), `"request_id":"other"`)
	resp = pageRequest(t, server, fmt.Sprintf("/api/v1/records/%d", other.ID), 7)
	require.Equal(t, 404, resp.StatusCode)
	role.Store(10)
	resp = pageRequest(t, server, "/api/v1/records", 7)
	data, err = io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Contains(t, string(data), `"request_id":"other"`)
	role.Store(1)
	resp = pageRequest(t, server, fmt.Sprintf("/api/v1/records/%d", other.ID), 7)
	require.Equal(t, 404, resp.StatusCode, "role downgrade must apply immediately")
}

func TestIdentityForgeryExpirationAndDelegationMismatch(t *testing.T) {
	_, server, _ := fixture(t)
	for _, test := range []struct {
		name     string
		token    func(*http.Request) string
		delegate string
		status   int
	}{
		{"missing", func(*http.Request) string { return "" }, "delegate-7", 401},
		{"wrong_plugin", func(r *http.Request) string {
			return signedIdentity(r, 7, time.Now().Add(30*time.Second).Unix(), "other")
		}, "delegate-7", 401},
		{"expired", func(r *http.Request) string {
			return signedIdentity(r, 7, time.Now().Add(-time.Second).Unix(), "example")
		}, "delegate-7", 401},
		{"tamper", func(r *http.Request) string {
			return signedIdentity(r, 7, time.Now().Add(30*time.Second).Unix(), "example") + "bad"
		}, "delegate-7", 401},
		{"delegation_mismatch", func(r *http.Request) string {
			return signedIdentity(r, 7, time.Now().Add(30*time.Second).Unix(), "example")
		}, "delegate-8", 403},
	} {
		t.Run(test.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", server.URL+"/api/v1/records", nil)
			require.NoError(t, err)
			req.Header.Set(middleware.IdentityHeader, test.token(req))
			req.Header.Set(hostclient.AuthorizationHeader, test.delegate)
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, test.status, resp.StatusCode)
		})
	}
	for _, path := range []string{"/internal/manifest", "/internal/runtime"} {
		resp, err := server.Client().Get(server.URL + path)
		require.NoError(t, err)
		require.Equal(t, 401, resp.StatusCode)
		resp.Body.Close()
	}
}

func TestRequestRecordsDebugPayloadAndRejection(t *testing.T) {
	s, server, _ := fixture(t)
	for _, test := range []struct {
		id, content string
		debug       bool
		rejected    bool
	}{{"debug", "private", true, false}, {"info", "private", false, false}, {"reject", "BLOCK", true, true}} {
		body, _ := json.Marshal(sdk.Request{Metadata: sdk.Metadata{RequestID: test.id, UserID: 7, TokenID: 1, Model: "m", Stream: true, Debug: test.debug, Attempt: 1}, Body: json.RawMessage(`{"model":"m","messages":[{"content":"` + test.content + `"}]}`)})
		req, err := http.NewRequest("POST", server.URL+"/internal/relay/request", bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set(sdk.ServiceHeader, secret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		require.NoError(t, err)
		var result sdk.RequestResult
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		resp.Body.Close()
		require.Equal(t, test.rejected, result.Error != nil)
		var record model.Record
		require.NoError(t, s.Store.DB.Where("request_id = ?", test.id).First(&record).Error)
		if test.debug {
			require.Contains(t, record.Input, test.content)
		} else {
			require.Empty(t, record.Input)
			require.Empty(t, record.Output)
		}
		if !test.rejected && test.debug {
			require.Contains(t, record.Output, "safe")
		}
		if test.rejected {
			require.Equal(t, "error", record.Status)
			require.Equal(t, "reject", record.Action)
		}
	}
}

func TestRealDuplexSSEProcessingAndFinalRecord(t *testing.T) {
	s, server, _ := fixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reader, writer := io.Pipe()
	defer writer.Close()
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/internal/relay/response", reader)
	require.NoError(t, err)
	req.Header.Set(sdk.ServiceHeader, secret)
	encoder := json.NewEncoder(writer)
	begin := make(chan error, 1)
	go func() {
		begin <- encoder.Encode(sdk.Frame{Type: "begin", Metadata: &sdk.Metadata{RequestID: "stream", UserID: 7, Model: "m", Stream: true, Debug: true, Attempt: 1}})
	}()
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.NoError(t, <-begin)
	decoder := sdk.NewDecoder(resp.Body)
	var ready sdk.Frame
	require.NoError(t, decoder.Decode(&ready))
	require.Equal(t, "ready", ready.Type)
	inputs := [][]byte{[]byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"sec\"}}]}\n\n"), []byte("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ret\"}}]}\n\n"), []byte("data: {\"choices\":[],\"usage\":{\"total_tokens\":9}}\n\n"), []byte("data: [DONE]\n\n")}
	var outputs []sdk.Unit
	for i, data := range inputs {
		sent := make(chan error, 1)
		go func() { sent <- encoder.Encode(sdk.Frame{Type: "data", Unit: &sdk.Unit{Sequence: i + 1, Data: data}}) }()
		var ack sdk.Frame
		require.NoError(t, decoder.Decode(&ack))
		require.NoError(t, <-sent)
		require.Equal(t, "ack", ack.Type)
		outputs = append(outputs, ack.Outputs...)
	}
	end := make(chan error, 1)
	go func() { end <- encoder.Encode(sdk.Frame{Type: "end"}); writer.Close() }()
	var done sdk.Frame
	require.NoError(t, decoder.Decode(&done))
	require.NoError(t, <-end)
	require.Equal(t, "end", done.Type)
	outputs = append(outputs, done.Outputs...)
	require.Len(t, outputs, len(inputs))
	var content strings.Builder
	for i, unit := range outputs {
		require.NoError(t, sdk.ValidateOutput(inputs[i], unit.Data, true))
		object, err := sdk.DecodeObject(sdk.EventData(unit.Data))
		if err == nil {
			for _, field := range sdk.ResponseText(object) {
				content.WriteString(field.Text())
			}
		}
	}
	require.Equal(t, "redacted", content.String())
	var record model.Record
	require.NoError(t, s.Store.DB.Where("request_id = ?", "stream").First(&record).Error)
	require.Equal(t, "completed", record.Status)
	require.Equal(t, 4, record.InputEvents)
	require.Equal(t, 4, record.OutputEvents)
	require.True(t, record.Terminal)
	require.NotContains(t, record.Output, "[DONE]")
	require.NotEmpty(t, record.Output)
}

func TestRuntimeCallbackPersistsAndRejectsInvalidOrigin(t *testing.T) {
	s, server, _ := fixture(t)
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte("host:\n  base_url: "+s.Host.HostBaseURL()+"\n"), 0600))
	s.Host.ConfigPath = path
	for _, value := range []string{"http://callback.example:3000", "http://bad.example/api"} {
		data, _ := json.Marshal(map[string]string{"host_url": value, "data_host_url": "http://data.example:3001"})
		req, err := http.NewRequest("PUT", server.URL+"/internal/runtime", bytes.NewReader(data))
		require.NoError(t, err)
		req.Header.Set(sdk.ServiceHeader, secret)
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		if strings.HasSuffix(value, "/api") {
			require.Equal(t, 400, resp.StatusCode)
		} else {
			require.Equal(t, 200, resp.StatusCode)
		}
	}
	require.Equal(t, "http://callback.example:3000", s.Host.HostBaseURL())
	require.Equal(t, "http://data.example:3001", s.Host.DataHostBaseURL())
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "http://callback.example:3000")
	require.Contains(t, string(data), "http://data.example:3001")
}

func TestDataCallbackHeaderDoesNotChangeControlCallback(t *testing.T) {
	s, server, _ := fixture(t)
	body, err := json.Marshal(sdk.Request{Metadata: sdk.Metadata{RequestID: "data-header", UserID: 7, Attempt: 1}, Body: json.RawMessage(`{"model":"m","messages":[{"content":"hello"}]}`)})
	require.NoError(t, err)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/internal/relay/request", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(sdk.ServiceHeader, secret)
	req.Header.Set("X-AIHub-Data-Host-Base-URL", "http://slave.example:3001")
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	resp.Body.Close()
	require.Equal(t, 200, resp.StatusCode)
	require.NotEqual(t, "http://slave.example:3001", s.Host.HostBaseURL())
	require.NotEqual(t, "http://slave.example:3001", s.Host.DataHostBaseURL())
}

func TestRecordDetailChecksResponseStageOutsideCurrentPage(t *testing.T) {
	s, server, _ := fixture(t)
	request := model.Record{UserID: 7, RequestID: "linked", Point: sdk.RequestPoint, Status: "completed"}
	require.NoError(t, s.Store.DB.Create(&request).Error)
	read := func() bool {
		resp := pageRequest(t, server, fmt.Sprintf("/api/v1/records/%d", request.ID), 7)
		var result struct {
			Data struct {
				ResponseExecuted bool `json:"response_executed"`
			}
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&result))
		return result.Data.ResponseExecuted
	}
	require.False(t, read())
	// An unauthorized user's identical request ID must not affect this result.
	require.NoError(t, s.Store.DB.Create(&model.Record{UserID: 8, RequestID: "linked", Point: sdk.ResponsePoint}).Error)
	require.False(t, read())
	require.NoError(t, s.Store.DB.Create(&model.Record{UserID: 7, RequestID: "linked", Point: sdk.ResponsePoint}).Error)
	require.True(t, read())
}
