<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { pnlClass } from "../pnlClass";

type Item = {
  product_code: string;
  name: string;
  market_value: string;
  cumulative: string;
  daily_pnl: string;
  display_date: string;
  latest_nav: string;
  latest_nav_date: string;
  hang_zero: boolean;
  stale_days: number;
  listed: boolean;
};

const data = ref<{
  market_value: string;
  cumulative_pnl: string;
  daily_pnl: string;
  mixed_dates: boolean;
  natural_day: string;
  items: Item[];
} | null>(null);
const err = ref("");

onMounted(async () => {
  try {
    data.value = await api("/api/overview");
  } catch (e) {
    err.value = e instanceof Error ? e.message : "加载失败";
  }
});
</script>

<template>
  <div v-if="err" class="err">{{ err }}</div>
  <template v-else-if="data">
    <p class="muted">自然日 {{ data.natural_day }}</p>
    <div class="grid">
      <div class="card"><h3>总市值</h3><div class="num">{{ data.market_value }}</div></div>
      <div class="card"><h3>总累计收益</h3><div class="num" :class="pnlClass(data.cumulative_pnl)">{{ data.cumulative_pnl }}</div></div>
      <div class="card">
        <h3>总当日收益（各账户展示日之和）</h3>
        <div class="num" :class="pnlClass(data.daily_pnl)">{{ data.daily_pnl }}</div>
        <p v-if="data.mixed_dates" class="tag">各产品结算日可能不同</p>
      </div>
    </div>
    <h2>持仓账户</h2>
    <table>
      <thead>
        <tr>
          <th>产品</th>
          <th>市值</th>
          <th>累计收益</th>
          <th>当日收益 / 展示日</th>
          <th>最新净值 / 净值日</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="it in data.items" :key="it.product_code">
          <td>
            <router-link :to="'/holdings/' + it.product_code">{{ it.name }}</router-link>
            <div class="muted mono">{{ it.product_code }}</div>
          </td>
          <td class="mono">{{ it.market_value }}</td>
          <td class="mono" :class="pnlClass(it.cumulative)">{{ it.cumulative }}</td>
          <td>
            <div class="mono" :class="pnlClass(it.daily_pnl)">{{ it.daily_pnl }}</div>
            <div class="muted">展示日 {{ it.display_date || "—" }}</div>
          </td>
          <td>
            <div class="mono">{{ it.latest_nav }}</div>
            <div class="muted">净值日 {{ it.latest_nav_date || "—" }} · 间隔 {{ it.stale_days }} 天</div>
            <span v-if="it.hang_zero" class="tag">净值未更新</span>
            <span v-if="!it.listed" class="tag">已不在代销目录</span>
          </td>
        </tr>
        <tr v-if="!data.items?.length"><td colspan="5" class="muted">还没有持仓。去建仓。</td></tr>
      </tbody>
    </table>
  </template>
</template>
