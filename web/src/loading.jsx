const sdk = window.AIHubPluginSDK;
const React = sdk.React;
const { Box, Stack, Skeleton, LinearProgress, Alert, Button, TableRow, TableCell } = sdk.MUI;

export function PluginSkeleton({ variant = "detail", minHeight = 180 }) {
  const Shared = sdk.components?.RegionSkeleton;
  if (Shared) return <Shared variant={variant} minHeight={minHeight} />;
  return (
    <Stack
      aria-hidden="true"
      spacing={2}
      sx={{ p: 2, minHeight, "@media (prefers-reduced-motion: reduce)": { "& .MuiSkeleton-root": { animation: "none" } } }}
    >
      {[70, 90, 65, 80].map((width, i) => (
        <Skeleton key={i} animation="pulse" variant="rounded" height={variant === "form" ? 40 : 28} width={`${width}%`} />
      ))}
    </Stack>
  );
}

function FallbackLoading({ loading = false, hasData, error, onRetry, skeleton = "detail", minHeight = 180, children }) {
  const translatedRetry = sdk.i18n?.t("observability.retry");
  const retryLabel =
    translatedRetry && translatedRetry !== "observability.retry"
      ? translatedRetry
      : { zh_CN: "重试", zh_HK: "重試", ja_JP: "再試行", en_US: "Retry" }[sdk.i18n?.language] || "Retry";
  const complete = React.useRef(false);
  const ready = hasData ?? (complete.current || (!loading && !error));
  React.useEffect(() => {
    if (!loading && !error && hasData !== false) {
      complete.current = true;
    }
  }, [loading, error, hasData]);
  const initial = !ready && (loading || hasData === false) && !error;
  const hidden = skeleton !== false && (initial || (error && !ready));
  return (
    <Box aria-busy={loading || initial} sx={{ position: "relative", minHeight }}>
      {error && !loading && (
        <Alert
          severity="error"
          action={
            onRetry && (
              <Button size="small" variant="outlined" onClick={onRetry}>
                {retryLabel}
              </Button>
            )
          }
        >
          {error}
        </Alert>
      )}
      <Box sx={{ display: "grid" }}>
        <Box
          inert={loading || initial ? "" : undefined}
          aria-hidden={hidden || undefined}
          sx={{
            gridArea: "1 / 1",
            visibility: hidden ? "hidden" : "visible",
            ...(error && !ready ? { height: 0, overflow: "hidden" } : {})
          }}
        >
          {children}
        </Box>
        {initial && !error && skeleton !== false && (
          <Box role="status" sx={{ gridArea: "1 / 1" }}>
            {typeof skeleton === "string" ? <PluginSkeleton variant={skeleton} minHeight={minHeight} /> : skeleton}
          </Box>
        )}
      </Box>
      {loading && ready && (
        <LinearProgress
          sx={{
            position: "absolute",
            inset: "0 0 auto",
            height: 2,
            "@media (prefers-reduced-motion: reduce)": { "& *": { animation: "none" } }
          }}
        />
      )}
    </Box>
  );
}

export function PluginLoading(props) {
  const Shared = sdk.components?.DataLoadingBoundary || FallbackLoading;
  return <Shared {...props} />;
}

export function PluginTableRows({ colSpan = 6, rows = 6 }) {
  const Shared = sdk.components?.TableLoadingRows;
  if (Shared) return <Shared colSpan={colSpan} rows={rows} />;
  return Array.from({ length: rows }, (_, i) => (
    <TableRow key={i} aria-busy="true">
      {Array.from({ length: colSpan }, (_, j) => (
        <TableCell key={j} sx={{ height: 53, "@media (prefers-reduced-motion: reduce)": { "& .MuiSkeleton-root": { animation: "none" } } }}>
          <Skeleton animation="pulse" width={j ? "60%" : "80%"} />
        </TableCell>
      ))}
    </TableRow>
  ));
}
