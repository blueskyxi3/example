import { PluginLoading, PluginTableRows } from "./loading.jsx";
import { translate, useTranslation } from "./i18n.js";
import { payloadText, queryParams, shouldPoll } from "./view-model.mjs";

const sdk = window.AIHubPluginSDK;
const React = sdk.React;
const { useEffect, useRef, useState } = React;
const { Alert, Box, Button, Dialog, DialogContent, DialogTitle, IconButton, MenuItem, Stack, SvgIcon, Table, TableBody, TableCell, TableContainer, TableHead, TablePagination, TableRow, TextField, Tooltip, Typography } = sdk.MUI;
const emptyFilters = { request_id: "", point: "", status: "", from: "", to: "" };
const pointName = (point, t) => t(point === "relay.request.intercept" ? "request" : "response");
const statusName = (status, t) => t(status === "error" ? "failed" : status);
const iconPaths = {
  detail: "M8 3h8l5 5v12a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h3zm8 0v5h5M8 13h8M8 17h6",
  input: "M3 12h11m-4-4 4 4-4 4M17 4h2a2 2 0 0 1 2 2v12a2 2 0 0 1-2 2h-2",
  output: "M21 12H10m4-4-4 4 4 4M7 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h2",
  close: "M18 6 6 18M6 6l12 12"
};

function DetailIcon({ name }) {
  return <Box component="svg" viewBox="0 0 24 24" aria-hidden="true" sx={{ width: 20, height: 20, flexShrink: 0, fill: "none", stroke: "currentColor", strokeWidth: 1.8, strokeLinecap: "round", strokeLinejoin: "round" }}><path d={iconPaths[name]} /></Box>;
}

async function request(path, config = {}) {
  const bases = sdk.getPluginBases("example");
  if (!bases) throw new Error(translate("error"));
  const response = await sdk.API.get(bases.apiBase + path, { suppressGlobalError: true, ...config });
  if (!response.data.success) throw new Error(response.data.message || translate("error"));
  return response.data.data;
}

export function topNavigation() {
  return [{ id: "records", title: translate("title"), url: "/plugins/example", icon: "tabler:plug-connected", before: "about" }];
}

function Payload({ record, name, t }) {
  return <Box sx={{ minWidth: 0 }}>
    <Stack direction="row" alignItems="center" justifyContent="space-between" spacing={1} sx={{ mb: 1 }}>
      <Stack direction="row" alignItems="center" spacing={0.75} sx={{ color: "text.primary" }}>
        <DetailIcon name={name} />
        <Typography variant="subtitle2">{t(name === "output" ? "payloadOutput" : name)}</Typography>
      </Stack>
      <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: "nowrap" }}>{record[`${name}_events`]} {t("events")} · {record[`${name}_bytes`]} {t("bytes")}</Typography>
    </Stack>
    <Box component="pre" sx={{ m: 0, p: 1.5, minHeight: 120, maxHeight: 320, overflow: "auto", bgcolor: "background.default", border: 1, borderColor: "divider", borderRadius: 1, whiteSpace: "pre-wrap", overflowWrap: "anywhere", fontSize: 12 }}>{payloadText(record, name, t("noBody"))}</Box>
    {record[`${name}_truncated`] && <Typography variant="caption" color="warning.main">{t("truncated")}</Typography>}
  </Box>;
}

function RequestID({ value, onFilter, t }) {
  const [copyStatus, setCopyStatus] = useState("copyRequestID");
  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopyStatus("copied");
      return;
    } catch {
      const textarea = document.createElement("textarea");
      textarea.value = value;
      textarea.style.cssText = "position:fixed;left:-9999px;top:0";
      const focused = document.activeElement;
      document.body.appendChild(textarea);
      textarea.select();
      try { setCopyStatus(document.execCommand("copy") ? "copied" : "copyFailed"); }
      catch { setCopyStatus("copyFailed"); }
      finally { textarea.remove(); focused?.focus(); }
    }
  };
  return <Stack direction="row" alignItems="center" spacing={0.5} sx={{ minWidth: 0, "&:hover .request-copy-button, &:focus-within .request-copy-button": { opacity: 1, pointerEvents: "auto" } }}>
    <Button size="small" title={value} sx={{ minWidth: 0, maxWidth: 166, textTransform: "none", justifyContent: "flex-start" }} onClick={onFilter}><Box component="span" sx={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{value}</Box></Button>
    <Tooltip title={t(copyStatus)} placement="top" arrow onClose={() => setCopyStatus("copyRequestID")}>
      <IconButton className="request-copy-button" size="small" aria-label={t("copyRequestID")} onClick={copy} sx={{ flexShrink: 0, opacity: 0, pointerEvents: "none", transition: "opacity 0.15s ease", "@media (hover: none)": { opacity: 1, pointerEvents: "auto" } }}>
        <SvgIcon sx={{ fontSize: 13 }}><path d="M19 21H8V7h11m0-2H8a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h11a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2m-3-4H4a2 2 0 0 0-2 2v14h2V3h12V1Z" /></SvgIcon>
      </IconButton>
    </Tooltip>
  </Stack>;
}

function RecordsPage() {
  const t = useTranslation();
  const [draft, setDraft] = useState(emptyFilters);
  const [filters, setFilters] = useState(emptyFilters);
  const [page, setPage] = useState(0);
  const [size, setSize] = useState(20);
  const [data, setData] = useState({ items: [], total: 0, admin: false });
  const [loading, setLoading] = useState(true);
  const [hasRecords, setHasRecords] = useState(false);
  const [error, setError] = useState("");
  const [refresh, setRefresh] = useState(0);
  const [visible, setVisible] = useState(!document.hidden);
  const [detail, setDetail] = useState(null);
  const [detailLoading, setDetailLoading] = useState(false);
  const [detailReady, setDetailReady] = useState(false);
  const [detailError, setDetailError] = useState("");
  const detailRequest = useRef(null);

  useEffect(() => {
    const change = () => { setVisible(!document.hidden); if (!document.hidden) setRefresh((v) => v + 1); };
    document.addEventListener("visibilitychange", change);
    return () => { document.removeEventListener("visibilitychange", change); detailRequest.current?.abort(); };
  }, []);

  useEffect(() => {
    const abort = new AbortController();
    setLoading(true);
    request("/records", { params: queryParams(filters, page, size), signal: abort.signal })
      .then((result) => { if (!abort.signal.aborted) { setData(result); setHasRecords(true); setError(""); } })
      .catch((err) => { if (!abort.signal.aborted) setError(err.message || t("error")); })
      .finally(() => { if (!abort.signal.aborted) setLoading(false); });
    return () => abort.abort();
  }, [filters, page, size, refresh]);

  useEffect(() => {
    if (error || !shouldPoll(data.items, visible)) return undefined;
    const timer = setTimeout(() => setRefresh((v) => v + 1), 2000);
    return () => clearTimeout(timer);
  }, [data, visible, error]);

  const apply = (next) => { setLoading(true); setPage(0); setFilters({ ...next }); };
  const openDetail = async (row) => {
    detailRequest.current?.abort();
    const abort = new AbortController(); detailRequest.current = abort;
    setDetail(row); setDetailLoading(true); setDetailReady(false); setDetailError("");
    try { const result = await request(`/records/${row.id}`, { signal: abort.signal }); if (!abort.signal.aborted) { setDetail(result); setDetailReady(true); } }
    catch (err) { if (!abort.signal.aborted) setDetailError(err.message || t("error")); }
    finally { if (!abort.signal.aborted) setDetailLoading(false); }
  };
  const closeDetail = () => { detailRequest.current?.abort(); setDetail(null); setDetailLoading(false); };
  const hasResponse = detail?.response_executed;

  return <Box sx={{ width: "100%", maxWidth: 1440, mx: "auto", p: { xs: 2, md: 3 } }}>
    <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
      <Typography variant="h3">{t("title")}</Typography>
      <Typography variant="caption" color="text.secondary">{t("scope")}: {t(data.admin ? "admin" : "own")}</Typography>
    </Stack>
    <Stack direction="row" alignItems="center" useFlexGap flexWrap="wrap" spacing={1} sx={{ mb: 2, "& .MuiInputBase-root": { height: "var(--app-control-height, 32px)", minHeight: "var(--app-control-height, 32px)" }, "& .MuiInputBase-input": { py: 0 } }}>
      <TextField size="small" placeholder={t("requestID")} inputProps={{ "aria-label": t("requestID") }} value={draft.request_id} onChange={(e) => setDraft({ ...draft, request_id: e.target.value })} sx={{ minWidth: 220, flex: "1 1 220px" }} />
      <TextField select size="small" value={draft.point} onChange={(e) => setDraft({ ...draft, point: e.target.value })} SelectProps={{ displayEmpty: true, inputProps: { "aria-label": t("point") } }} sx={{ minWidth: 180 }}>
        <MenuItem value="">{t("point")}: {t("all")}</MenuItem>
        <MenuItem value="relay.request.intercept">{t("request")}</MenuItem><MenuItem value="relay.response.intercept">{t("response")}</MenuItem>
      </TextField>
      <TextField select size="small" value={draft.status} onChange={(e) => setDraft({ ...draft, status: e.target.value })} SelectProps={{ displayEmpty: true, inputProps: { "aria-label": t("status") } }} sx={{ minWidth: 130 }}>
        <MenuItem value="">{t("status")}: {t("all")}</MenuItem>
        {["running", "completed", "error", "canceled"].map((value) => <MenuItem key={value} value={value}>{statusName(value, t)}</MenuItem>)}
      </TextField>
      {["from", "to"].map((name) => <TextField key={name} size="small" type="datetime-local" placeholder={t(name)} inputProps={{ "aria-label": t(name), title: t(name) }} value={draft[name]} onChange={(e) => setDraft({ ...draft, [name]: e.target.value })} sx={{ minWidth: 190 }} />)}
      <Button variant="outlined" size="small" sx={{ height: "var(--app-control-height, 32px)", minHeight: "var(--app-control-height, 32px)" }} onClick={() => apply(draft)}>{t("search")}</Button>
    </Stack>
    <Box sx={{ minHeight: 42 }}>{error && <Alert severity="error">{error}</Alert>}</Box>
    <PluginLoading loading={loading} hasData={hasRecords} error={error} onRetry={() => setRefresh((v) => v + 1)} skeleton={false}><TableContainer sx={{ border: 1, borderColor: "divider", borderRadius: 1 }}>
      <Table size="small" sx={{
        minWidth: 980,
        "& .MuiTableHead-root, & .MuiTableHead-root .MuiTableCell-root": { bgcolor: "background.paper" },
        "& .MuiTableHead-root > .MuiTableRow-root > .MuiTableCell-root, & .MuiTableBody-root > .MuiTableRow-root > .MuiTableCell-root": {
          borderBottom: "1px solid", borderRight: "1px solid", borderColor: "divider"
        },
        "& .MuiTableRow-root > .MuiTableCell-root:last-of-type": { borderRight: 0 },
        "& .MuiTableBody-root > .MuiTableRow-root:last-of-type > .MuiTableCell-root": { borderBottom: 0 }
      }}>
        <TableHead><TableRow>{["requestID", "point", "model", "input", "processing", "output", "status", "duration", "details"].map((key) => <TableCell key={key} sx={{ whiteSpace: "nowrap" }}>{t(key)}</TableCell>)}</TableRow></TableHead>
        <TableBody>
          {error && !hasRecords ? null : loading && !hasRecords ? <PluginTableRows colSpan={9} /> : data.items.length === 0 ? <TableRow><TableCell colSpan={9} align="center" sx={{ height: 180 }}>{t("empty")}</TableCell></TableRow> : data.items.map((row) => <TableRow key={row.id} hover>
            <TableCell sx={{ maxWidth: 200 }}><RequestID value={row.request_id} t={t} onFilter={() => { const next = { ...emptyFilters, request_id: row.request_id }; setDraft(next); apply(next); }} /><Typography variant="caption" display="block" color="text.secondary">{new Date(row.created_at * 1000).toLocaleString()} · {t("user")} {row.user_id}</Typography></TableCell>
            <TableCell>{pointName(row.point, t)}<Typography variant="caption" display="block" color="text.secondary">{t(row.stream ? "stream" : "json")} · {t("attempt")} {row.attempt}</Typography></TableCell>
            <TableCell sx={{ maxWidth: 180, overflowWrap: "anywhere" }}>{row.model || "—"}<Typography variant="caption" display="block" color="text.secondary">{row.channel_id || "—"}</Typography></TableCell>
            <TableCell>{row.input_events} {t("events")}<Typography variant="caption" display="block" color="text.secondary">{row.input_bytes} {t("bytes")}</Typography></TableCell>
            <TableCell>{t(row.action)}<Typography variant="caption" display="block" color="text.secondary">{t("hits")}: {row.hits}</Typography></TableCell>
            <TableCell>{row.output_events} {t("events")}<Typography variant="caption" display="block" color="text.secondary">{row.output_bytes} {t("bytes")}</Typography></TableCell>
            <TableCell>{statusName(row.status, t)}</TableCell>
            <TableCell sx={{ whiteSpace: "nowrap" }}>{row.duration_ms} ms</TableCell>
            <TableCell><Button size="small" onClick={() => openDetail(row)}>{t("details")}</Button></TableCell>
          </TableRow>)}
        </TableBody>
      </Table>
    </TableContainer></PluginLoading>
    <TablePagination component="div" count={data.total} page={page} rowsPerPage={size} rowsPerPageOptions={[20, 50, 100]} labelDisplayedRows={hasRecords ? undefined : () => "—"} labelRowsPerPage={t("pagination")} onPageChange={(_, value) => { setLoading(true); setPage(value); }} onRowsPerPageChange={(e) => { setLoading(true); setSize(Number(e.target.value)); setPage(0); }} />
    <Dialog open={Boolean(detail)} onClose={closeDetail} fullWidth maxWidth="md" aria-labelledby="example-detail-title">
      <DialogTitle id="example-detail-title" sx={{ px: 3, py: 1.5, borderBottom: 1, borderColor: "divider" }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between">
          <Stack direction="row" alignItems="center" spacing={1}>
            <Box sx={{ color: "primary.main", display: "flex" }}><DetailIcon name="detail" /></Box>
            <Typography variant="h5" component="span">{t("details")}</Typography>
          </Stack>
          <IconButton aria-label={t("close")} onClick={closeDetail} size="small" sx={{ color: "text.secondary" }}><DetailIcon name="close" /></IconButton>
        </Stack>
      </DialogTitle>
      <DialogContent sx={{ px: 3, pb: 2.5 }}>
        {detail && <Stack spacing={1.5} sx={{ pt: 2.5 }}>
          <Typography variant="body2" sx={{ overflowWrap: "anywhere" }}>{detail.request_id} · {pointName(detail.point, t)} · {statusName(detail.status, t)}</Typography>
          <Typography variant="caption" color="text.secondary">{t("processing")}: {t(detail.action)} · {t("hits")}: {detail.hits} · {t("duration")}: {detail.duration_ms} ms{detail.error_code ? ` · ${detail.error_code}` : ""}</Typography>
          {detail.stream && <Typography variant="caption" color="text.secondary">{t("debugNote")} {t("terminal")}: {t(detail.terminal ? "yes" : "no")}</Typography>}
          {!detailLoading && detail.point === "relay.request.intercept" && hasResponse === false && <Alert severity="info">{t("responseNotRun")} <Button size="small" onClick={() => { const next = { ...emptyFilters, request_id: detail.request_id }; setDraft(next); apply(next); }}>{t("requestLink")}</Button></Alert>}
          <PluginLoading key={detail.id} loading={detailLoading} hasData={detailReady} error={detailError} onRetry={() => openDetail(detail)} skeleton="detail"><Box sx={{ display: "grid", gridTemplateColumns: { xs: "1fr", sm: "1fr 1fr" }, gap: 2, pt: 0.5 }}><Payload record={detail} name="input" t={t} /><Payload record={detail} name="output" t={t} /></Box></PluginLoading>
        </Stack>}
      </DialogContent>
    </Dialog>
  </Box>;
}

export function Page({ path = "" }) {
  const t = useTranslation();
  return path ? <Alert severity="info">{t("notFound")}</Alert> : <RecordsPage />;
}
