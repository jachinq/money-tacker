<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { api } from "../api";
import { pnlClass } from "../pnlClass";

type CatalogItem = {
  product_code: string;
  name: string;
  issuer: string;
  listed: boolean;
  latest_nav: string;
  latest_nav_date: string;
  holding: "none" | "open" | "closed";
  ret_30: string | null;
  ret_90: string | null;
  ret_180: string | null;
  ret_360: string | null;
  ret_730: string | null;
};

const q = ref("");
const includeUnlisted = ref(false);
const sort = ref("ret_360");
const order = ref<"asc" | "desc">("desc");
const page = ref(1);
const pageSize = 50;
const total = ref(0);
const items = ref<CatalogItem[]>([]);
const empty = ref("");
const loading = ref(false);
const err = ref("");
let seq = 0;

const windows = [
  { key: "ret_30", label: "30 日", sort: "ret_30" },
  { key: "ret_90", label: "90 日", sort: "ret_90" },
  { key: "ret_180", label: "180 日", sort: "ret_180" },
  { key: "ret_360", label: "360 日", sort: "ret_360" },
  { key: "ret_730", label: "两年", sort: "ret_730" },
] as const;

const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

async function load() {
  const n = ++seq;
  loading.value = true;
  err.value = "";
  const params = new URLSearchParams();
  if (q.value.trim()) params.set("q", q.value.trim());
  if (includeUnlisted.value) params.set("include_unlisted", "true");
  params.set("sort", sort.value);
  params.set("order", order.value);
  params.set("page", String(page.value));
  params.set("page_size", String(pageSize));
  try {
    const d = await api<{
      items: CatalogItem[];
      total: number;
      empty?: string;
    }>("/api/products?" + params.toString());
    if (n !== seq) return;
    items.value = d.items || [];
    total.value = d.total || 0;
    empty.value = d.empty || "";
  } catch (e) {
    if (n !== seq) return;
    err.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    if (n === seq) loading.value = false;
  }
}

function clickSort(key: string) {
  if (sort.value === key) {
    order.value = order.value === "desc" ? "asc" : "desc";
  } else {
    sort.value = key;
    order.value = key.startsWith("ret_") || key === "listed" ? "desc" : "asc";
  }
  page.value = 1;
  load();
}

function thClass(key: string) {
  return sort.value === key ? "sort-on" : "";
}

function holdingLabel(h: CatalogItem["holding"]) {
  if (h === "open") return "持有中";
  if (h === "closed") return "已清仓";
  return "从未开户";
}

function holdingTo(it: CatalogItem) {
  if (it.holding === "none") {
    return { path: "/holdings/new", query: { product_code: it.product_code } };
  }
  return "/holdings/" + it.product_code;
}

watch(includeUnlisted, () => {
  page.value = 1;
  load();
});

let qTimer = 0;
watch(q, () => {
  window.clearTimeout(qTimer);
  qTimer = window.setTimeout(() => {
    page.value = 1;
    load();
  }, 250);
});

onMounted(load);
onUnmounted(() => window.clearTimeout(qTimer));
</script>

<template>
  <div>
    <h2>产品目录</h2>
    <p class="muted">
      窗口净值涨跌幅按单位净值计算，终点是该产品最新净值日，与是否持仓无关。采集开始日之前没有历史，不够长的窗口显示为 —。
    </p>
    <div class="catalog-bar">
      <input v-model="q" placeholder="代码 / 名称 / 发行机构" />
      <label><input type="checkbox" v-model="includeUnlisted" /> 含不在架</label>
    </div>
    <p v-if="err" class="err">{{ err }}</p>
    <p v-else-if="!loading && empty === 'no_products'" class="muted">库中还没有产品。需要先成功跑完一次中行代销目录采集。</p>
    <p v-else-if="!loading && items.length === 0" class="muted">没有匹配的产品。</p>
    <div v-else class="table-scroll">
      <table>
        <thead>
          <tr>
            <th class="sortable" :class="thClass('code')" @click="clickSort('code')">代码</th>
            <th class="sortable" :class="thClass('name')" @click="clickSort('name')">名称</th>
            <th class="sortable" :class="thClass('issuer')" @click="clickSort('issuer')">发行机构</th>
            <th class="sortable" :class="thClass('listed')" @click="clickSort('listed')">在架</th>
            <th class="sortable" :class="thClass('latest_nav_date')" @click="clickSort('latest_nav_date')">最新净值</th>
            <th v-for="w in windows" :key="w.key" :class="['mono', 'sortable', thClass(w.sort)]" @click="clickSort(w.sort)">
              {{ w.label }}
            </th>
            <th>持仓</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="it in items" :key="it.product_code">
            <td class="mono">{{ it.product_code }}</td>
            <td>{{ it.name }}</td>
            <td>{{ it.issuer }}</td>
            <td>{{ it.listed ? "在架" : "不在架" }}</td>
            <td>
              <div class="mono">{{ it.latest_nav || "—" }}</div>
              <div class="muted">{{ it.latest_nav_date || "—" }}</div>
            </td>
            <td v-for="w in windows" :key="w.key" class="mono" :class="pnlClass(String(it[w.key] ?? ''))">
              {{ it[w.key] || "—" }}
            </td>
            <td>
              <router-link :to="holdingTo(it)">{{ holdingLabel(it.holding) }}</router-link>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="total > pageSize" class="catalog-pager">
      <button class="btn ghost" type="button" :disabled="page <= 1" @click="page--; load()">上一页</button>
      <span class="muted">{{ page }} / {{ pages }} · 共 {{ total }}</span>
      <button class="btn ghost" type="button" :disabled="page >= pages" @click="page++; load()">下一页</button>
    </div>
  </div>
</template>
