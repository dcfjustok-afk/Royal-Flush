import { createRouter, createWebHistory } from "vue-router";

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: "/", name: "lobby", component: () => import("@/views/LobbyView.vue") },
    { path: "/account", name: "account", component: () => import("@/views/AccountView.vue") },
    { path: "/invite/:code", name: "invite", component: () => import("@/views/InviteView.vue") },
    { path: "/rooms/new", name: "create-room", component: () => import("@/views/CreateRoomView.vue") },
    { path: "/rooms/:id/waiting", name: "waiting-room", component: () => import("@/views/WaitingRoomView.vue") },
    { path: "/rooms/:id/table", name: "table", component: () => import("@/views/TableView.vue"), meta: { immersive: true } },
    { path: "/profile", name: "profile", component: () => import("@/views/ProfileView.vue") },
    { path: "/:pathMatch(.*)*", redirect: "/" },
  ],
  scrollBehavior: () => ({ top: 0 }),
});

