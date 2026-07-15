// js/auth.js

export async function initAuth() {
  await window.wasmReady;
  return await window.initAuthGo();
}

export async function logout() {
  await window.wasmReady;
  return await window.logoutGo();
}

export async function showSetupScreen(status) {
  await window.wasmReady;
  return window.showSetupScreenGo(status);
}

export async function showLoginScreen(status) {
  await window.wasmReady;
  return window.showLoginScreenGo(status);
}

export async function showAppContainer() {
  await window.wasmReady;
  return window.showAppContainerGo();
}

export async function sanitizeHTML(html) {
  await window.wasmReady;
  return window.sanitizeHTMLGo(html);
}
