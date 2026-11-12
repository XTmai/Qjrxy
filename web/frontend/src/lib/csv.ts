// CSV 解析：支持引号包裹、字段内逗号/换行、转义双引号与 BOM。
// 后端导出的 cookies 列是 JSON 字符串，会落在引号里，必须走状态机而不是 split(',')。

/** 把 CSV 文本解析成「表头 -> 值」的对象数组。 */
export function parseCSV(text: string): Record<string, string>[] {
  let src = text;
  // Excel 保存的 UTF-8 CSV 带 BOM
  if (src.charCodeAt(0) === 0xfeff) src = src.slice(1);

  const rows: string[][] = [];
  let row: string[] = [];
  let field = "";
  let inQuotes = false;

  for (let i = 0; i < src.length; i++) {
    const ch = src[i];
    if (inQuotes) {
      if (ch === '"') {
        if (src[i + 1] === '"') {
          field += '"';
          i++;
        } else {
          inQuotes = false;
        }
      } else {
        field += ch;
      }
      continue;
    }
    if (ch === '"') {
      inQuotes = true;
    } else if (ch === ",") {
      row.push(field);
      field = "";
    } else if (ch === "\n") {
      row.push(field);
      rows.push(row);
      row = [];
      field = "";
    } else if (ch !== "\r") {
      field += ch;
    }
  }
  if (field !== "" || row.length > 0) {
    row.push(field);
    rows.push(row);
  }
  if (rows.length === 0) return [];

  const header = rows[0].map((h) => h.trim());
  return rows
    .slice(1)
    .filter((r) => r.some((v) => v.trim() !== ""))
    .map((r) => {
      const obj: Record<string, string> = {};
      header.forEach((key, idx) => {
        if (key) obj[key] = (r[idx] ?? "").trim();
      });
      return obj;
    });
}
