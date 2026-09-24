export type DayRow = {
  date: string;
  pnl: string;
  hang_zero: boolean;
  has_nav: boolean;
  unit_nav?: string;
};

export type CellKind = "pad" | "blank" | "hang_zero" | "flat" | "pos" | "neg";

export type MonthCell = {
  key: string;
  kind: CellKind;
  day?: number;
  date?: string;
  pnl?: string;
  unitNav?: string;
  isDisplayDate?: boolean;
};

export function shanghaiTodayISO(now = new Date()): string {
  return new Intl.DateTimeFormat("en-CA", { timeZone: "Asia/Shanghai" }).format(now);
}

export function shiftMonth(year: number, month: number, delta: number): { year: number; month: number } {
  const d = new Date(Date.UTC(year, month - 1 + delta, 1));
  return { year: d.getUTCFullYear(), month: d.getUTCMonth() + 1 };
}

export function monthTitle(year: number, month: number): string {
  return `${year}年${month}月`;
}

function pad2(n: number): string {
  return n < 10 ? "0" + n : String(n);
}

function isoDate(year: number, month: number, day: number): string {
  return `${year}-${pad2(month)}-${pad2(day)}`;
}

function mondayOffset(year: number, month: number): number {
  const jsDay = new Date(Date.UTC(year, month - 1, 1)).getUTCDay();
  return (jsDay + 6) % 7;
}

function daysInMonth(year: number, month: number): number {
  return new Date(Date.UTC(year, month, 0)).getUTCDate();
}

function kindForRow(row: DayRow): CellKind {
  if (row.hang_zero) return "hang_zero";
  const n = Number(row.pnl);
  if (n > 0) return "pos";
  if (n < 0) return "neg";
  return "flat";
}

export function buildMonthCells(opts: {
  year: number;
  month: number;
  days: DayRow[];
  today: string;
  displayDate?: string;
}): MonthCell[] {
  const byDate = new Map(opts.days.map((d) => [d.date, d]));
  const offset = mondayOffset(opts.year, opts.month);
  const last = daysInMonth(opts.year, opts.month);
  const out: MonthCell[] = [];

  for (let i = 0; i < offset; i++) {
    out.push({ key: `pad-lead-${i}`, kind: "pad" });
  }

  for (let day = 1; day <= last; day++) {
    const date = isoDate(opts.year, opts.month, day);
    const isDisplayDate = !!opts.displayDate && opts.displayDate === date;
    if (date > opts.today) {
      out.push({ key: date, kind: "blank", day, date, isDisplayDate });
      continue;
    }
    const row = byDate.get(date);
    if (!row) {
      out.push({ key: date, kind: "blank", day, date, isDisplayDate });
      continue;
    }
    const unitNav = row.hang_zero || !row.has_nav ? "无净值" : row.unit_nav;
    out.push({
      key: date,
      kind: kindForRow(row),
      day,
      date,
      pnl: row.pnl,
      unitNav,
      isDisplayDate,
    });
  }

  const rem = out.length % 7;
  if (rem !== 0) {
    for (let i = 0; i < 7 - rem; i++) {
      out.push({ key: `pad-tail-${i}`, kind: "pad" });
    }
  }
  return out;
}

export function cellClass(cell: MonthCell): string {
  const parts = ["cal-cell"];
  if (cell.kind === "pad") parts.push("cal-cell-pad");
  if (cell.kind === "blank") parts.push("cal-cell-blank");
  if (cell.kind === "hang_zero") parts.push("cal-cell-hang");
  if (cell.kind === "flat") parts.push("cal-cell-flat");
  if (cell.kind === "pos") parts.push("cal-cell-pos");
  if (cell.kind === "neg") parts.push("cal-cell-neg");
  if (cell.isDisplayDate) parts.push("cal-cell-display");
  return parts.join(" ");
}
