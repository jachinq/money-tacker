import assert from "node:assert/strict";
import { test } from "node:test";
import { buildMonthCells, shiftMonth } from "./calendarMonth.ts";

test("September 2026 Monday grid: 1st is Tuesday so one leading pad", () => {
  const cells = buildMonthCells({
    year: 2026,
    month: 9,
    days: [],
    today: "2026-09-21",
  });
  assert.equal(cells.length % 7, 0);
  assert.equal(cells[0].kind, "pad");
  assert.equal(cells[1].day, 1);
  assert.equal(cells[1].kind, "blank");
});

test("hang-zero, published flat, pos, and future stay distinct", () => {
  const cells = buildMonthCells({
    year: 2026,
    month: 9,
    today: "2026-09-21",
    displayDate: "2026-09-19",
    days: [
      { date: "2026-09-18", pnl: "0.00", hang_zero: true, has_nav: false },
      { date: "2026-09-19", pnl: "1.25", hang_zero: false, has_nav: true, unit_nav: "1.0562" },
      { date: "2026-09-20", pnl: "0.00", hang_zero: false, has_nav: true, unit_nav: "1.0562" },
      { date: "2026-09-22", pnl: "3.00", hang_zero: false, has_nav: true, unit_nav: "1.0600" },
    ],
  });
  const byDay = new Map(cells.filter((c) => c.day).map((c) => [c.day, c]));
  assert.equal(byDay.get(18)?.kind, "hang_zero");
  assert.equal(byDay.get(18)?.pnl, "0.00");
  assert.equal(byDay.get(18)?.unitNav, "无净值");
  assert.equal(byDay.get(19)?.kind, "pos");
  assert.equal(byDay.get(19)?.unitNav, "1.0562");
  assert.equal(byDay.get(19)?.isDisplayDate, true);
  assert.equal(byDay.get(20)?.kind, "flat");
  assert.equal(byDay.get(21)?.kind, "blank");
  assert.equal(byDay.get(22)?.kind, "blank");
  assert.equal(byDay.get(22)?.pnl, undefined);
  assert.equal(byDay.get(22)?.unitNav, undefined);
  assert.equal(byDay.get(21)?.unitNav, undefined);
});

test("shiftMonth wraps the year", () => {
  assert.deepEqual(shiftMonth(2026, 1, -1), { year: 2025, month: 12 });
  assert.deepEqual(shiftMonth(2025, 12, 1), { year: 2026, month: 1 });
});
