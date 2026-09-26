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
        <router-link to="/crawls">采集</router-link>
        <router-link to="/holdings">持仓</router-link>
        <router-link to="/holdings/new">建仓</router-link>
        <router-link to="/account">{{ s.account }}</router-link>
      </nav>
    </header>
    <router-view />
  </div>
</template>
