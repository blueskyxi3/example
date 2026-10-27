import assert from "node:assert/strict";
import test from "node:test";
import { payloadText, queryParams, shouldPoll } from "./view-model.mjs";

test("poll only while visible requests remain running", () => {
  assert.equal(shouldPoll([{ status: "running" }], true), true);
  assert.equal(shouldPoll([{ status: "running" }], false), false);
  assert.equal(shouldPoll([{ status: "completed" }, { status: "error" }], true), false);
});
test("non-debug details never render payloads", () => {
  assert.equal(payloadText({ debug: false, input: "secret" }, "input", "hidden"), "hidden");
  assert.equal(payloadText({ debug: true, output: "last content" }, "output", "hidden"), "last content");
});
test("filters retain exact request id and timestamps", () => {
  const filters = { request_id: "req-1", point: "relay.response.intercept", status: "error", from: "2026-10-11T00:00:00Z", to: "" };
  assert.deepEqual(queryParams(filters, 2, 20), { page: 2, size: 20, request_id: "req-1", point: "relay.response.intercept", status: "error", from: 1791676800 });
});
