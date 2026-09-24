export function productCodeClickNotices(input: {
  copied: boolean;
  hasCatalogPage: boolean;
  opened: boolean;
}): string[] {
  const notices = [input.copied ? "已复制" : "复制失败"];
  if (!input.hasCatalogPage) notices.push("没有代销目录页");
  else if (!input.opened) notices.push("页面没打开");
  return notices;
}
