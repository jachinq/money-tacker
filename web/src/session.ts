import { defineStore } from "pinia";
import { ref } from "vue";
import { api } from "./api";

export const useSession = defineStore("session", () => {
  const account = ref<string | null>(null);
  const loaded = ref(false);

  async function refresh() {
    try {
      const me = await api<{ account: string }>("/api/me");
      account.value = me.account;
    } catch {
      account.value = null;
    } finally {
      loaded.value = true;
    }
  }

  return { account, loaded, refresh };
});
