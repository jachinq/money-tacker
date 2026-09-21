<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { api, ApiError } from "../api";

const product_code = ref("AF247494G");
const amount = ref("10000");
const occur_date = ref("");
const unit_nav = ref("");
const err = ref("");
const router = useRouter();

async function submit() {
  err.value = "";
  try {
    await api("/api/holdings", {
      method: "POST",
      body: JSON.stringify({
        product_code: product_code.value,
        amount: amount.value,
        occur_date: occur_date.value || undefined,
        unit_nav: unit_nav.value || undefined,
      }),
    });
    router.push("/holdings/" + product_code.value.toUpperCase());
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "失败";
  }
}
</script>

<template>
  <div>
    <h2>首次买入</h2>
    <form class="form" @submit.prevent="submit">
      <label>产品代码 <input v-model="product_code" required /></label>
      <label>购入金额（元） <input v-model="amount" required /></label>
      <label>发生日（可空=今天） <input v-model="occur_date" placeholder="YYYY-MM-DD" /></label>
      <label>手工净值（可选） <input v-model="unit_nav" /></label>
      <p v-if="err" class="err">{{ err }}</p>
      <button class="btn" type="submit">建仓</button>
    </form>
  </div>
</template>
