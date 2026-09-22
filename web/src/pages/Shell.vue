<script setup lang="ts">
import { useSession } from "../session";
const s = useSession();
</script>

<template>
  <div class="wrap">
    <header class="top">
      <div class="brand">理财收益追踪</div>
      <nav>
        <router-link to="/">总览</router-link>
        <router-link to="/products">产品</router-link>
        <router-link to="/holdings">持仓</router-link>
        <router-link to="/holdings/new">建仓</router-link>
        <router-link to="/account">{{ s.account }}</router-link>
      </nav>
    </header>
    <p class="disclaimer">收益估算，以发行机构/代销机构结算为准。当日收益对应展示日（不晚于昨天的最近净值日），不是打开页面的自然日。</p>
    <router-view />
  </div>
</template>
