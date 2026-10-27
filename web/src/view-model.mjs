export function shouldPoll(records, visible) {
  return visible && records.some((record) => record.status === "running");
}
export function payloadText(record, field, noBody) {
  return record.debug ? record[field] || "—" : noBody;
}
export function queryParams(filters, page, size) {
  const result = { page, size };
  for (const name of ["request_id", "point", "status"]) if (filters[name]) result[name] = filters[name];
  for (const name of ["from", "to"]) if (filters[name]) result[name] = Math.floor(new Date(filters[name]).getTime() / 1000);
  return result;
}
