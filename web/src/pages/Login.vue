<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { api, ApiError } from "../api";
import { useSession } from "../session";

const account = ref("");
const password = ref("");
const err = ref("");
const router = useRouter();
const session = useSession();

async function submit() {
  err.value = "";
  try {
    await api("/api/auth/login", { method: "POST", body: JSON.stringify({ account: account.value, password: password.value }) });
    await session.refresh();
    router.push("/");
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "登录失败";
  }
}
</script>

<template>
  <div class="wrap">
    <h1>登录</h1>
    <form class="form" @submit.prevent="submit">
      <label>用户名 <input v-model="account" autocomplete="username" /></label>
      <label>密码 <input v-model="password" type="password" autocomplete="current-password" /></label>
      <p v-if="err" class="err">{{ err }}</p>
      <button class="btn" type="submit">进入</button>
      <router-link to="/register">没有账号？注册</router-link>
    </form>
  </div>
</template>
