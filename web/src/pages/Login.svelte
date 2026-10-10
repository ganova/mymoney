<script lang="ts">
  import { api } from '../lib/api';
  import { loadBootstrap, toastError } from '../lib/state.svelte';

  let password = $state('');
  let busy = $state(false);

  async function login(e: Event) {
    e.preventDefault();
    busy = true;
    try {
      await api.post('/auth/login', { password });
      await loadBootstrap();
    } catch (err) {
      toastError(err);
    } finally {
      busy = false;
    }
  }
</script>

<div class="login">
  <form class="card stack" onsubmit={login}>
    <div class="center">
      <div class="logo">💰</div>
      <h1>MyMoney</h1>
      <p class="muted small">Keuangan pribadi, dicatat secepat chat.</p>
    </div>
    <label class="field">
      <span>Kata sandi</span>
      <!-- svelte-ignore a11y_autofocus -->
      <input class="input" type="password" bind:value={password} autocomplete="current-password" autofocus required />
    </label>
    <button class="btn primary block" disabled={busy || !password}>
      {#if busy}<span class="spinner"></span>{/if} Masuk
    </button>
  </form>
</div>

<style>
  .login { min-height: 100dvh; display: grid; place-items: center; padding: 24px 16px; }
  .card { width: 100%; max-width: 380px; padding: 28px 20px; }
  .logo { font-size: 52px; }
  h1 { font-size: 26px; font-weight: 750; margin-top: 4px; }
</style>
