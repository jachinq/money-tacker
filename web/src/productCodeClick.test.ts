import assert from "node:assert/strict";
import { test } from "node:test";
import { productCodeClickNotices } from "./productCodeClick.ts";

test("copied and catalog tab opened", () => {
  assert.deepEqual(productCodeClickNotices({ copied: true, hasCatalogPage: true, opened: true }), ["已复制"]);
});

test("copy failed but catalog tab still opened", () => {
  assert.deepEqual(productCodeClickNotices({ copied: false, hasCatalogPage: true, opened: true }), ["复制失败"]);
});

test("no catalog page still reports the copy", () => {
  assert.deepEqual(productCodeClickNotices({ copied: true, hasCatalogPage: false, opened: false }), ["已复制", "没有代销目录页"]);
});

test("popup blocked keeps the copy notice", () => {
  assert.deepEqual(productCodeClickNotices({ copied: true, hasCatalogPage: true, opened: false }), ["已复制", "页面没打开"]);
});
