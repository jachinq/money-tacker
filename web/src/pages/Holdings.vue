<script setup lang="ts">
import { onMounted, ref } from "vue";
import { api } from "../api";
import { pnlClass } from "../pnlClass";

const items = ref<any[]>([]);
const closed = ref(false);
async function load() {
  const q = closed.value ? "?include_closed=true" : "";
  const d = await api<{ items: any[] }>("/api/holdings" + q);
  items.value = d.items || [];
}
onMounted(load);
</script>

<template>
  <div>
    <h2>持仓账户</h2>
    <label><input type="checkbox" v-model="closed" @change="load" /> 含已清仓</label>
    <div class="table-scroll">
    <table>
      <thead><tr><th>产品</th><th>剩余成本</th><th>市值</th><th>未实现</th><th>累计</th><th>累计收益率</th><th>年化累计收益率</th><th>当日收益</th></tr></thead>
      <tbody>
        <tr v-for="it in items" :key="it.product_code">
          <td>
            <router-link :to="'/holdings/' + it.product_code">{{ it.name }}</router-link>
            <div class="muted mono">{{ it.product_code }}</div>
          </td>
          <td class="mono">{{ it.cost }}</td>
          <td class="mono">{{ it.market_value }}</td>
          <td class="mono" :class="pnlClass(it.unrealized)">{{ it.unrealized }}</td>
          <td class="mono" :class="pnlClass(it.cumulative)">{{ it.cumulative }}</td>
          <td class="mono" :class="pnlClass(it.cumulative_return || '0')">{{ it.cumulative_return ? it.cumulative_return + "%" : "—" }}</td>
          <td class="mono" :class="pnlClass(it.annualized_cumulative_return || '0')">{{ it.annualized_cumulative_return ? it.annualized_cumulative_return + "%" : "—" }}</td>
          <td>
            <div class="mono" :class="pnlClass(it.daily_pnl)">{{ it.daily_pnl }}</div>
            <div class="muted">展示日 {{ it.display_date || "—" }} · 净值日 {{ it.latest_nav_date || "—" }}</div>
          </td>
        </tr>
      </tbody>
    </table>
    </div>
  </div>
</template>
