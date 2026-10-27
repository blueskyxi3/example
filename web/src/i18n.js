const sdk = window.AIHubPluginSDK;
const messages = {
  en_US: {
    copyRequestID: "Copy request ID", copied: "Copied", copyFailed: "Copy failed",
    title: "Plugin example", requestID: "Request ID", point: "Extension point", status: "Status", from: "From", to: "To",
    request: "After API key authentication", response: "After channel response", all: "All", search: "Search",
    user: "User", model: "Model / channel", input: "Input", processing: "Processing", output: "Output", payloadOutput: "Output", duration: "Duration", details: "Details",
    loading: "Loading…", empty: "No execution records", error: "Unable to load records", close: "Close", notFound: "Page not found",
    running: "Running", completed: "Completed", canceled: "Canceled", interrupted: "Interrupted", failed: "Error", pass: "Pass through", replace: "Replace", reject: "Reject",
    noBody: "The current log level does not save payloads", truncated: "Snapshot truncated", stream: "Stream", json: "JSON", attempt: "Attempt", hits: "Rule matches",
    events: "events", bytes: "bytes", terminal: "Terminal event", yes: "Received", no: "Not received", responseNotRun: "Response hook not executed yet / request ended before channel response",
    debugNote: "Stream details show the last content event, not a full replay.", own: "Your records", admin: "All users", scope: "Visibility", requestLink: "Show this request", pagination: "Rows per page",
  },
  zh_CN: {
    copyRequestID: "复制请求 ID", copied: "已复制", copyFailed: "复制失败",
    title: "插件示例", requestID: "请求 ID", point: "扩展点", status: "状态", from: "开始时间", to: "结束时间",
    request: "API Key 鉴权后", response: "渠道返回后", all: "全部", search: "搜索",
    user: "用户", model: "模型 / 渠道", input: "输入", processing: "处理", output: "返回", payloadOutput: "输出", duration: "耗时", details: "详情",
    loading: "加载中…", empty: "暂无执行记录", error: "记录加载失败", close: "关闭", notFound: "页面不存在",
    running: "执行中", completed: "已完成", canceled: "已取消", interrupted: "已中断", failed: "错误", pass: "原样透传", replace: "替换", reject: "拒绝",
    noBody: "当前日志级别未保存正文", truncated: "快照已截断", stream: "流式", json: "JSON", attempt: "渠道尝试", hits: "规则命中次数",
    events: "事件", bytes: "字节", terminal: "结束事件", yes: "已接收", no: "未接收", responseNotRun: "响应扩展点尚未执行，或请求在渠道返回前结束",
    debugNote: "流式详情展示末次有效内容事件，不提供完整回放。", own: "本人记录", admin: "全部用户", scope: "查看范围", requestLink: "查看此请求", pagination: "每页记录数",
  },
  zh_HK: {
    copyRequestID: "複製請求 ID", copied: "已複製", copyFailed: "複製失敗",
    title: "插件示例", requestID: "請求 ID", point: "擴展點", status: "狀態", from: "開始時間", to: "結束時間",
    request: "API Key 驗證後", response: "渠道返回後", all: "全部", search: "搜尋",
    user: "使用者", model: "模型 / 渠道", input: "輸入", processing: "處理", output: "返回", payloadOutput: "輸出", duration: "耗時", details: "詳情",
    loading: "載入中…", empty: "暫無執行記錄", error: "記錄載入失敗", close: "關閉", notFound: "頁面不存在",
    running: "執行中", completed: "已完成", canceled: "已取消", interrupted: "已中斷", failed: "錯誤", pass: "原樣透傳", replace: "替換", reject: "拒絕",
    noBody: "目前日誌級別未儲存正文", truncated: "快照已截斷", stream: "串流", json: "JSON", attempt: "渠道嘗試", hits: "規則命中次數",
    events: "事件", bytes: "位元組", terminal: "結束事件", yes: "已接收", no: "未接收", responseNotRun: "回應擴展點尚未執行，或請求在渠道返回前結束",
    debugNote: "串流詳情展示最後有效內容事件，不提供完整回放。", own: "本人記錄", admin: "全部使用者", scope: "查看範圍", requestLink: "查看此請求", pagination: "每頁記錄數",
  },
  ja_JP: {
    copyRequestID: "リクエスト ID をコピー", copied: "コピーしました", copyFailed: "コピーに失敗しました",
    title: "プラグイン例", requestID: "リクエスト ID", point: "拡張ポイント", status: "状態", from: "開始日時", to: "終了日時",
    request: "API キー認証後", response: "チャネル応答後", all: "すべて", search: "検索",
    user: "ユーザー", model: "モデル / チャネル", input: "入力", processing: "処理", output: "出力", payloadOutput: "出力", duration: "所要時間", details: "詳細",
    loading: "読込中…", empty: "実行記録がありません", error: "記録の読込に失敗しました", close: "閉じる", notFound: "ページが見つかりません",
    running: "実行中", completed: "完了", canceled: "キャンセル", interrupted: "中断", failed: "エラー", pass: "そのまま通過", replace: "置換", reject: "拒否",
    noBody: "現在のログレベルでは本文を保存しません", truncated: "スナップショットを切り詰めました", stream: "ストリーム", json: "JSON", attempt: "試行", hits: "ルール一致数",
    events: "イベント", bytes: "バイト", terminal: "終了イベント", yes: "受信済み", no: "未受信", responseNotRun: "応答フックは未実行、またはチャネル応答前に終了しました",
    debugNote: "ストリーム詳細には最後の内容イベントを表示します。全体の再生はできません。", own: "自分の記録", admin: "全ユーザー", scope: "表示範囲", requestLink: "このリクエストを表示", pagination: "ページあたりの件数",
  },
};
function locale() {
  const language = sdk.i18n?.language || "zh_CN";
  if (language.startsWith("en")) return "en_US";
  if (language.startsWith("ja")) return "ja_JP";
  return /HK|TW|Hant/i.test(language) ? "zh_HK" : "zh_CN";
}
export const translate = (key) => messages[locale()][key] || key;
export function useTranslation() {
  sdk.useTranslation();
  return translate;
}
