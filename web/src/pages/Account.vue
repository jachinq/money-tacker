<script setup lang="ts">
import { ref } from "vue";
import { useRouter } from "vue-router";
import { api, ApiError } from "../api";
import { useSession } from "../session";

const old_password = ref("");
const new_password = ref("");
const err = ref("");
const ok = ref("");
const router = useRouter();
const session = useSession();

async function save() {
  err.value = "";
  ok.value = "";
  try {
    await api("/api/auth/password", {
      method: "POST",
      body: JSON.stringify({ old_password: old_password.value, new_password: new_password.value }),
    });
    ok.value = "已更新";
  } catch (e) {
    err.value = e instanceof ApiError ? e.message : "失败";
  }
}

async function logout() {
  await api("/api/auth/logout", { method: "POST", body: "{}" });
  await session.refresh();
  router.push("/login");
}
</script>

<template>
  <div>
    <h2>账号 {{ session.account }}</h2>
    <form class="form" @submit.prevent="save">
      <label>原密码 <input v-model="old_password" type="password" /></label>
      <label>新密码 <input v-model="new_password" type="password" /></label>
      <p v-if="err" class="err">{{ err }}</p>
      <p v-if="ok">{{ ok }}</p>
      <button class="btn" type="submit">修改密码</button>
    </form>
    <p><button class="btn ghost" @click="logout">退出</button></p>
  </div>
</template>
