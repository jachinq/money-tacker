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
    await api("/api/auth/register", { method: "POST", body: JSON.stringify({ account: account.value, password: password.value }) });
    await session.refresh();
    router.push("/");
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "注册失败";
  }
}
</script>

<template>
  <div class="wrap">
    <h1>注册</h1>
    <p class="muted">库中尚无用户时可注册第一个账号；之后需将 REGISTER_OPEN 设为 true 并重启。</p>
    <form class="form" @submit.prevent="submit">
      <label>用户名 <input v-model="account" autocomplete="username" /></label>
      <label>密码（至少 6 位） <input v-model="password" type="password" autocomplete="new-password" /></label>
      <p v-if="err" class="err">{{ err }}</p>
      <button class="btn" type="submit">创建账号</button>
      <router-link to="/login">已有账号</router-link>
    </form>
  </div>
</template>
