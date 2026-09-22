import { createRouter, createWebHistory } from "vue-router";
import { useSession } from "./session";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/login", component: () => import("./pages/Login.vue") },
    { path: "/register", component: () => import("./pages/Register.vue") },
    {
      path: "/",
      component: () => import("./pages/Shell.vue"),
      children: [
        { path: "", component: () => import("./pages/Overview.vue") },
        { path: "products", component: () => import("./pages/Products.vue") },
        { path: "holdings", component: () => import("./pages/Holdings.vue") },
        { path: "holdings/new", component: () => import("./pages/NewHolding.vue") },
        { path: "holdings/:code", component: () => import("./pages/HoldingDetail.vue") },
        { path: "account", component: () => import("./pages/Account.vue") },
      ],
    },
  ],
});

router.beforeEach(async (to) => {
  const s = useSession();
  if (!s.loaded) await s.refresh();
  const pub = to.path === "/login" || to.path === "/register";
  if (!s.account && !pub) return "/login";
  if (s.account && pub) return "/";
});
