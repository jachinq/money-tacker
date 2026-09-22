<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue";
import { RouterLink, useRoute, useRouter } from "vue-router";
import { api, ApiError } from "../api";

type ProductView = {
  product_code: string;
  name: string;
  listed: boolean;
  latest_nav: string;
  latest_nav_date: string;
};

const product_code = ref("AF247494G");
const amount = ref("10000");
const occur_date = ref("");
const unit_nav = ref("");
const err = ref("");
const name = ref("");
const listed = ref<boolean | null>(null);
const latest_nav = ref("");
const latest_nav_date = ref("");
const existing = ref(false);
const verifiedCode = ref("");
const looking = ref(false);
const router = useRouter();
const route = useRoute();

let lookupSeq = 0;
let inflightCode = "";

function normCode(s: string) {
  return s.trim().toUpperCase();
}

function clearPreview() {
  name.value = "";
  listed.value = null;
  latest_nav.value = "";
  latest_nav_date.value = "";
  existing.value = false;
}

watch(product_code, (v) => {
  if (normCode(v) === verifiedCode.value) return;
  verifiedCode.value = "";
  clearPreview();
  err.value = "";
});

const listedLabel = computed(() => {
  if (listed.value === null) return "";
  return listed.value ? "在代销目录" : "不在代销目录";
});

const navDisplay = computed(() => {
  if (verifiedCode.value !== normCode(product_code.value)) return "";
  return latest_nav.value || "—";
});

const navDateDisplay = computed(() => {
  if (verifiedCode.value !== normCode(product_code.value)) return "";
  return latest_nav_date.value || "—";
});

const canSubmit = computed(() => {
  const code = normCode(product_code.value);
  if (!code || verifiedCode.value !== code || looking.value) return false;
  if (!listed.value || existing.value) return false;
  if (!latest_nav.value && !unit_nav.value.trim()) return false;
  return true;
});

async function lookup() {
  const code = normCode(product_code.value);
  if (!code) {
    lookupSeq++;
    looking.value = false;
    inflightCode = "";
    verifiedCode.value = "";
    clearPreview();
    err.value = "";
    return;
  }
  if (verifiedCode.value === code) return;
  if (looking.value && inflightCode === code) return;

  looking.value = true;
  inflightCode = code;
  const seq = ++lookupSeq;
  err.value = "";
  try {
    const p = await api<ProductView>("/api/products/" + encodeURIComponent(code));
    if (seq !== lookupSeq) return;
    let hasHolding = false;
    try {
      await api("/api/holdings/" + encodeURIComponent(code));
      hasHolding = true;
    } catch (e) {
      if (!(e instanceof ApiError) || e.status !== 404) throw e;
    }
    if (seq !== lookupSeq) return;
    product_code.value = p.product_code || code;
    name.value = p.name || "";
    listed.value = !!p.listed;
    latest_nav.value = p.latest_nav || "";
    latest_nav_date.value = p.latest_nav_date || "";
    existing.value = hasHolding;
    verifiedCode.value = product_code.value;
    if (hasHolding) {
      err.value = "该产品已有持仓账户，请走追加买入";
    } else if (!p.listed) {
      err.value = "产品已不在代销目录，不能建仓";
    } else if (!latest_nav.value && !unit_nav.value.trim()) {
      err.value = "无可用净值，请填写手工净值";
    }
  } catch (e) {
    if (seq !== lookupSeq) return;
    verifiedCode.value = "";
    clearPreview();
    err.value = e instanceof ApiError ? e.message : "查询失败";
  } finally {
    if (seq === lookupSeq) {
      looking.value = false;
      inflightCode = "";
    }
  }
}

onMounted(() => {
  const code = String(route.query.product_code || "").trim();
  if (code) {
    product_code.value = code;
    lookup();
  }
});

async function submit() {
  err.value = "";
  if (!canSubmit.value) {
    if (normCode(product_code.value) !== verifiedCode.value) {
      err.value = "请先查询并确认产品";
    }
    return;
  }
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
      <label>
        产品代码
        <div class="code-row">
          <input v-model="product_code" required @blur="lookup" />
          <button class="btn ghost" type="button" :disabled="looking" @click="lookup">
            {{ looking ? "查询中" : "查询" }}
          </button>
        </div>
      </label>
      <label>产品名称 <input :value="name" readonly disabled /></label>
      <label>在架 <input :value="listedLabel" readonly disabled /></label>
      <label>最新净值 <input :value="navDisplay" readonly disabled /></label>
      <label>净值日 <input :value="navDateDisplay" readonly disabled /></label>
      <label>购入金额（元） <input v-model="amount" required /></label>
      <label>发生日（可空=今天） <input v-model="occur_date" placeholder="YYYY-MM-DD" /></label>
      <label>手工净值（可选） <input v-model="unit_nav" /></label>
      <p v-if="existing && verifiedCode" class="muted">
        <RouterLink :to="'/holdings/' + verifiedCode">打开该持仓账户追加买入</RouterLink>
      </p>
      <p v-if="err" class="err">{{ err }}</p>
      <button class="btn" type="submit" :disabled="!canSubmit">建仓</button>
    </form>
  </div>
</template>
