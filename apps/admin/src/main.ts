import "@fontsource/noto-sans-sc/400.css";
import "@fontsource/noto-sans-sc/500.css";
import "@fontsource/noto-sans-sc/700.css";
import "@fontsource/barlow-condensed/600.css";
import { createApp } from "vue";
import App from "./App.vue";
import { initializeTheme } from "./theme";
import "./styles.css";

initializeTheme();
createApp(App).mount("#app");

