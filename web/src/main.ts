import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';

try {
  const theme = localStorage.getItem('mm_theme');
  if (theme === 'light' || theme === 'dark') document.documentElement.setAttribute('data-theme', theme);
} catch {
  /* abaikan */
}

const app = mount(App, { target: document.getElementById('app')! });

// Service worker hanya berjalan di konteks aman (HTTPS / localhost).
if ('serviceWorker' in navigator && window.isSecureContext) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => {});
  });
}

export default app;
