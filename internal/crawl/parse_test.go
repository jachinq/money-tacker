package crawl

import "testing"

const fixture = `
<html><body>
<table>
<tr><th>产品代码</th><th>产品名称</th><th>单位净值</th><th>累计净值</th><th>日净值增长率</th><th>截止日期</th></tr>
<tr>
  <td>AF247494G</td>
  <td>信银理财安盈象固收稳利一个月持有期60号理财产品</td>
  <td>1.0562</td>
  <td>1.0562</td>
  <td>0.02%</td>
  <td>2026/09/18</td>
</tr>
</table>
</body></html>`

func TestParseAF247494G(t *testing.T) {
	rows, err := ParsePage(fixture)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("len=%d", len(rows))
	}
	r := rows[0]
	if r.Code != "AF247494G" {
		t.Fatalf("code %s", r.Code)
	}
	if r.Name == "" || !contains(r.Name, "信银理财") {
		t.Fatalf("name %s", r.Name)
	}
	if r.NavDate != "2026-09-18" {
		t.Fatalf("nav date %s", r.NavDate)
	}
	if r.UnitNavE8 != 105620000 {
		t.Fatalf("nav %d", r.UnitNavE8)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || stringIndex(s, sub) >= 0)
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
