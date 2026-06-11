// js/auth.js

import { request, resolvePath } from './api.js';

export async function initAuth() {
  // Check URL query parameters first (OIDC callbacks)
  const urlParams = new URLSearchParams(window.location.search);
  const tokenVal = urlParams.get('accessToken') || urlParams.get('setToken');
  if (tokenVal) {
    localStorage.setItem('token', tokenVal);
    urlParams.delete('accessToken');
    urlParams.delete('setToken');
    urlParams.delete('state');
    const cleanUrl = window.location.pathname + (urlParams.toString() ? '?' + urlParams.toString() : '');
    window.history.replaceState({}, document.title, cleanUrl);
  }

  // Always check server status first
  let status = null;
  try {
    status = await request('GET', '/status');
  } catch (err) {
    console.error('Failed to fetch /status:', err);
  }

  // If server is not initialized, show setup screen
  if (status && !status.isInit) {
    showSetupScreen();
    return null;
  }

  const token = localStorage.getItem('token');
  if (!token) {
    showLoginScreen(status);
    return null;
  }

  try {
    const payload = await request('POST', '/api/authorize');
    showAppContainer();
    return payload;
  } catch (err) {
    console.error('Initial authorize failed:', err);
    localStorage.removeItem('token');
    showLoginScreen(status);
    return null;
  }
}

export function showSetupScreen() {
  document.getElementById('setup-screen').classList.remove('hidden');
  document.getElementById('login-screen').classList.add('hidden');
  document.getElementById('app-container').classList.add('hidden');

  const form = document.getElementById('setup-form');
  const errEl = document.getElementById('setup-error');
  const successEl = document.getElementById('setup-success');
  const btn = document.getElementById('setup-submit-btn');

  form.onsubmit = async (e) => {
    e.preventDefault();
    errEl.classList.add('hidden');
    successEl.classList.add('hidden');

    const username = document.getElementById('setup-username').value.trim() || 'root';
    const password = document.getElementById('setup-password').value;
    const password2 = document.getElementById('setup-password2').value;

    if (password !== password2) {
      errEl.textContent = 'Passwords do not match.';
      errEl.classList.remove('hidden');
      return;
    }
    if (password.length < 5) {
      errEl.textContent = 'Password must be at least 5 characters.';
      errEl.classList.remove('hidden');
      return;
    }

    btn.disabled = true;
    btn.textContent = 'Creating account...';

    try {
      await request('POST', '/init', { newRoot: { username, password } });

      successEl.textContent = 'Account created! Signing you in...';
      successEl.classList.remove('hidden');

      // Auto-login after setup
      const loginPayload = await request('POST', '/login', { username, password });
      if (loginPayload && loginPayload.user && loginPayload.user.token) {
        localStorage.setItem('token', loginPayload.user.token);
      }
      document.getElementById('setup-screen').classList.add('hidden');
      showAppContainer();
      // Trigger app initialization
      window.dispatchEvent(new CustomEvent('abs:authed', { detail: loginPayload }));
    } catch (err) {
      errEl.textContent = err.message || 'Failed to create account. Please try again.';
      errEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = 'Create Root Account';
    }
  };
}

export function showLoginScreen(status) {
  document.getElementById('login-screen').classList.remove('hidden');
  document.getElementById('setup-screen').classList.add('hidden');
  document.getElementById('app-container').classList.add('hidden');
  applyStatusToLoginScreen(status);
}

export function showAppContainer() {
  document.getElementById('login-screen').classList.add('hidden');
  document.getElementById('setup-screen').classList.add('hidden');
  document.getElementById('app-container').classList.remove('hidden');
}

async function applyStatusToLoginScreen(status) {
  try {
    if (!status) {
      status = await request('GET', '/status');
    }
    const methods = status.authMethods || ['local'];
    const localForm = document.getElementById('login-form');
    const oidcBtn = document.getElementById('oidc-btn');
    const divider = document.getElementById('login-divider');

    if (methods.includes('local')) {
      localForm.classList.remove('hidden');
    } else {
      localForm.classList.add('hidden');
    }

    if (methods.includes('openid')) {
      oidcBtn.classList.remove('hidden');
      divider.classList.remove('hidden');

      const oidcBtnText = document.getElementById('oidc-btn-text');
      if (oidcBtnText && status.authFormData && status.authFormData.authLoginCustomMessage) {
        oidcBtnText.textContent = status.authFormData.authLoginCustomMessage;
      } else if (oidcBtnText) {
        oidcBtnText.textContent = 'Sign in with OpenId';
      }

      oidcBtn.onclick = () => {
        const callbackUrl = encodeURIComponent(window.location.origin + window.location.pathname);
        window.location.href = resolvePath(`/auth/openid?redirect=${callbackUrl}`);
      };
    } else {
      oidcBtn.classList.add('hidden');
      divider.classList.add('hidden');
    }
  } catch (err) {
    console.error('Failed to load server status:', err);
  }
}

export async function logout() {
  try {
    await request('POST', '/logout');
  } catch (err) {
    console.warn('Logout server request failed:', err);
  }
  localStorage.removeItem('token');
  localStorage.removeItem('activeLibraryId');
  showLoginScreen(null);
}
