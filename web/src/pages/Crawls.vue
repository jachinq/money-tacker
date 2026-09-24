<script setup lang="ts">
import { computed, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ApiError, api } from "../api";

type CrawlItem = {
  id: number;
  started_at: string;
  finished_at: string | null;
  status: string;
  pages_ok: number;
  products_ok: number;
  summary: string;
};

const route = useRoute();
const router = useRouter();
const pageSize = 50;
const items = ref<CrawlItem[]>([]);
const total = ref(0);
const busy = ref(false);
const err = ref("");
const loading = ref(false);
const starting = ref(false);
const expanded = ref<Record<number, boolean>>({});
let seq = 0;

const statusLabel: Record<string, string> = {
  running: "进行中",
  success: "成功",
  partial: "部分成功",
  fail: "失败",
};

const page = computed(() => {
  const n = Number(route.query.page || 1);
  return Number.isInteger(n) && n >= 1 ? n : 1;
});
const problems = computed(() => route.query.problems === "1");
const pages = computed(() => Math.max(1, Math.ceil(total.value / pageSize)));

function fmt(iso: string | null) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  }).format(d);
}

function queryOf(nextPage: number, onlyProblems: boolean) {
  const q: Record<string, string> = {};
  if (nextPage > 1) q.page = String(nextPage);
  if (onlyProblems) q.problems = "1";
  return q;
}

async function load() {
  const n = ++seq;
  loading.value = true;
  err.value = "";
  const params = new URLSearchParams();
  params.set("page", String(page.value));
  if (problems.value) params.set("problems", "1");
  try {
    const d = await api<{ items: CrawlItem[]; total: number; busy: boolean }>("/api/crawl-runs?" + params.toString());
    if (n !== seq) return;
    items.value = d.items || [];
    total.value = d.total || 0;
    busy.value = !!d.busy;
  } catch (e) {
    if (n !== seq) return;
    err.value = e instanceof Error ? e.message : "加载失败";
  } finally {
    if (n === seq) loading.value = false;
  }
}

function go(nextPage: number, onlyProblems = problems.value) {
  router.push({ path: "/crawls", query: queryOf(nextPage, onlyProblems) });
}

async function start() {
  starting.value = true;
  err.value = "";
  try {
    await api("/api/crawl-runs", { method: "POST", body: "{}" });
    expanded.value = {};
    if (page.value !== 1 || problems.value) {
      await router.replace({ path: "/crawls", query: {} });
    } else {
      await load();
    }
  } catch (e) {
    if (e instanceof ApiError && e.status === 409) busy.value = true;
    err.value = e instanceof Error ? e.message : "开始采集失败";
  } finally {
    starting.value = false;
  }
}

watch(() => [route.query.page, route.query.problems], load, { immediate: true });
</script>

<template>
  <div>
    <h2>采集</h2>
    <p class="muted">一次采集是对中行代销目录的全量扫描。定时、启动补跑和手工触发记在同一份列表里。进行中要看是否结束，请自行刷新。</p>
    <div class="catalog-bar">
      <button class="btn" type="button" :disabled="busy || starting" @click="start">开始采集</button>
      <label class="check"><input type="checkbox" :checked="problems" @change="go(1, !problems)" /> 只看失败与部分成功</label>
    </div>
    <p v-if="err" class="err">{{ err }}</p>
    <p v-else-if="!loading && items.length === 0" class="muted">没有采集。</p>
    <div v-else class="table-scroll">
      <table>
        <thead>
          <tr>
            <th>开始</th>
            <th>结束</th>
            <th>状态</th>
            <th>成功页数</th>
            <th>成功产品数</th>
            <th>摘要</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="it in items" :key="it.id">
            <td class="mono">{{ fmt(it.started_at) }}</td>
            <td class="mono">{{ fmt(it.finished_at) }}</td>
            <td>{{ statusLabel[it.status] || it.status }}</td>
            <td class="mono">{{ it.pages_ok }}</td>
            <td class="mono">{{ it.products_ok }}</td>
            <td>
              <p :class="expanded[it.id] ? 'summary-full' : 'summary-clip'">{{ it.summary || "—" }}</p>
              <button v-if="it.summary" class="btn ghost" type="button" @click="expanded[it.id] = !expanded[it.id]">
                {{ expanded[it.id] ? "收起" : "展开" }}
              </button>
            </td>
          </tr>
        </tbody>
      </table>
    </div>
    <div v-if="total > pageSize" class="catalog-pager">
      <button class="btn ghost" type="button" :disabled="page <= 1" @click="go(page - 1)">上一页</button>
      <span class="muted">{{ page }} / {{ pages }} · 共 {{ total }}</span>
      <button class="btn ghost" type="button" :disabled="page >= pages" @click="go(page + 1)">下一页</button>
    </div>
  </div>
</template>
