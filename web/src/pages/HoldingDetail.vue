<script setup lang="ts">
import { onMounted, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { api, ApiError } from "../api";
import DailyPnlCalendar from "../components/DailyPnlCalendar.vue";
import { pnlClass } from "../pnlClass";

const route = useRoute();
const router = useRouter();
const code = route.params.code as string;
const holding = ref<any>(null);
const ledger = ref<any[]>([]);
const days = ref<any[]>([]);
const collectedDaily = ref("");
const collectionGap = ref("");
const err = ref("");
const amount = ref("");
const occur = ref("");
const nav = ref("");
const redeemMode = ref<"amount" | "shares" | "all">("amount");
const redeemVal = ref("");

async function load() {
  const d = await api<{ holding: any; ledger: any[] }>("/api/holdings/" + code);
  holding.value = d.holding;
  ledger.value = d.ledger || [];
  const p = await api<{ days: any[]; collected_daily?: string; collection_gap?: string }>("/api/holdings/" + code + "/pnl");
  days.value = p.days || [];
  collectedDaily.value = p.collected_daily || "";
  collectionGap.value = p.collection_gap || "";
}

onMounted(async () => {
  try {
    await load();
  } catch (e) {
    err.value = e instanceof Error ? e.message : "加载失败";
  }
});

async function buy() {
  err.value = "";
  try {
    await api("/api/holdings/" + code + "/buys", {
      method: "POST",
      body: JSON.stringify({ amount: amount.value, occur_date: occur.value || undefined, unit_nav: nav.value || undefined }),
    });
    amount.value = "";
    await load();
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "失败";
  }
}

async function redeem() {
  err.value = "";
  const body: any = { occur_date: occur.value || undefined, unit_nav: nav.value || undefined };
  if (redeemMode.value === "all") body.all = true;
  else if (redeemMode.value === "shares") body.shares = redeemVal.value;
  else body.amount = redeemVal.value;
  try {
    await api("/api/holdings/" + code + "/redeems", { method: "POST", body: JSON.stringify(body) });
    redeemVal.value = "";
    await load();
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "失败";
  }
}

async function voidLast() {
  const last = [...ledger.value].reverse().find((x) => !x.voided);
  if (!last) return;
  err.value = "";
  try {
    await api("/api/holdings/" + code + "/ledger/" + last.id + "/void", { method: "POST", body: "{}" });
    await load();
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "失败";
  }
}

async function remove() {
  if (!confirm("删除整本账户及其流水？净值库不受影响。")) return;
  await api("/api/holdings/" + code, { method: "DELETE" });
  router.push("/holdings");
}
</script>

<template>
  <div v-if="err" class="err">{{ err }}</div>
  <div v-if="holding">
    <h2>{{ holding.name }}</h2>
    <p class="muted mono">{{ holding.product_code }}</p>
    <p>
      最新净值 {{ holding.latest_nav }} · 净值日 {{ holding.latest_nav_date }} · 间隔 {{ holding.stale_days }} 天
      <span v-if="holding.hang_zero" class="tag">净值未更新</span>
      <span v-if="!holding.listed" class="tag">已不在代销目录</span>
    </p>
    <div class="grid">
      <div class="card"><h3>市值</h3><div class="num">{{ holding.market_value }}</div></div>
      <div class="card">
        <h3>累计收益</h3>
        <div class="num" :class="pnlClass(holding.cumulative)">{{ holding.cumulative }}</div>
        <div class="muted">
          未实现 <span class="mono" :class="pnlClass(holding.unrealized)">{{ holding.unrealized }}</span>
          · 已实现 <span class="mono" :class="pnlClass(holding.realized)">{{ holding.realized }}</span>
        </div>
      </div>
      <div class="card">
        <h3>当日收益</h3>
        <div class="num" :class="pnlClass(holding.daily_pnl)">{{ holding.daily_pnl }}</div>
        <div class="muted">展示日 {{ holding.display_date || "—" }}</div>
      </div>
    </div>

    <div class="row-actions">
      <input v-model="amount" placeholder="追加金额" />
      <input v-model="occur" placeholder="发生日 YYYY-MM-DD" />
      <input v-model="nav" placeholder="手工净值" />
      <button class="btn" :disabled="!holding.listed" @click="buy">追加买入</button>
    </div>
    <div class="row-actions">
      <select v-model="redeemMode">
        <option value="amount">按金额</option>
        <option value="shares">按份额</option>
        <option value="all">全部赎回</option>
      </select>
      <input v-if="redeemMode !== 'all'" v-model="redeemVal" placeholder="金额或份额" />
      <button class="btn ghost" @click="redeem">赎回</button>
      <button class="btn ghost" @click="voidLast">作废最后一笔</button>
      <button class="btn ghost" @click="remove">删除账户</button>
    </div>

    <h3>流水</h3>
    <table>
      <thead><tr><th>日</th><th>方向</th><th>金额</th><th>份额</th><th>所用净值</th><th>净值日</th></tr></thead>
      <tbody>
        <tr v-for="e in ledger" :key="e.id" :class="{ muted: e.voided }">
          <td>{{ e.occur_date }}</td>
          <td>{{ e.kind === "buy" ? "买入" : "赎回" }}{{ e.voided ? "（已作废）" : "" }}</td>
          <td class="mono">{{ e.cash }}</td>
          <td class="mono">{{ e.shares }}</td>
          <td class="mono">{{ e.unit_nav }}</td>
          <td>{{ e.nav_date_used }}</td>
        </tr>
      </tbody>
    </table>

    <h3>按日收益（自然日月历）</h3>
    <p class="muted">与上方「当日收益」口径不同：这里按自然日排列。挂零与已公布且为 0 不是同一格。实线圈为展示日。</p>
    <p v-if="collectionGap" class="muted">
      已采集按日合计 <span class="mono">{{ collectedDaily }}</span>
      · 采集缺口 <span class="mono" :class="pnlClass(collectionGap)">{{ collectionGap }}</span>
      （累计 − 已采集按日之和；缺口不是某一天赚到的钱，也不记入日历第一天）
    </p>
    <DailyPnlCalendar :days="days" :display-date="holding.display_date || ''" />
  </div>
</template>
