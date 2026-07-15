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
    window.serverStatus = status;
  } catch (err) {
    console.error('Failed to fetch /status:', err);
  }

  // If server is not initialized, show setup screen
  if (status && !status.isInit) {
    showSetupScreen(status);
    return null;
  }

  const token = localStorage.getItem('token');
  if (!token) {
    showLoginScreen(status);
    return null;
  }

  try {
    const payload = await request('POST', '/api/authorize');
    if (payload && payload.user && payload.user.isOldToken) {
      console.warn('User has old token. Forcing re-login.');
      localStorage.removeItem('token');
      showLoginScreen(status);
      
      const usernameInput = document.getElementById('username');
      if (usernameInput) usernameInput.value = payload.user.username || '';
      
      const authWarning = document.getElementById('login-auth-warning');
      if (authWarning) {
        authWarning.classList.remove('hidden');
        authWarning.classList.add('flex');
        
        const moreInfoLink = document.getElementById('login-auth-warning-more-info');
        if (moreInfoLink) {
          const userType = payload.user.type;
          if (userType === 'admin' || userType === 'root') {
            moreInfoLink.classList.remove('hidden');
          } else {
            moreInfoLink.classList.add('hidden');
          }
        }
      }
      return null;
    }
    showAppContainer();
    return payload;
  } catch (err) {
    console.error('Initial authorize failed:', err);
    localStorage.removeItem('token');
    showLoginScreen(status);
    return null;
  }
}

export function showSetupScreen(status) {
  document.getElementById('setup-screen').classList.remove('hidden');
  document.getElementById('login-screen').classList.add('hidden');
  document.getElementById('app-container').classList.add('hidden');

  if (status) {
    const configPathEl = document.getElementById('setup-config-path');
    const metadataPathEl = document.getElementById('setup-metadata-path');
    if (configPathEl) configPathEl.value = status.ConfigPath || '';
    if (metadataPathEl) metadataPathEl.value = status.MetadataPath || '';
  }

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
    btn.textContent = 'Initializing...';

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
      let errorMsg = err.message;
      try {
        const parsed = JSON.parse(err.message);
        if (parsed && parsed.error) {
          errorMsg = parsed.error;
        }
      } catch (_) {}
      errEl.textContent = errorMsg || 'Failed to create account. Please try again.';
      errEl.classList.remove('hidden');
      btn.disabled = false;
      btn.textContent = 'Submit';
    }
  };
}

export function showLoginScreen(status) {
  document.getElementById('login-screen').classList.remove('hidden');
  document.getElementById('setup-screen').classList.add('hidden');
  document.getElementById('app-container').classList.add('hidden');
  
  const authWarning = document.getElementById('login-auth-warning');
  if (authWarning) {
    authWarning.classList.add('hidden');
    authWarning.classList.remove('flex');
  }
  
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

    const customMessageEl = document.getElementById('login-custom-message');
    if (customMessageEl && status.authFormData && status.authFormData.authLoginCustomMessage) {
      customMessageEl.innerHTML = status.authFormData.authLoginCustomMessage;
      customMessageEl.classList.remove('hidden');
    } else if (customMessageEl) {
      customMessageEl.classList.add('hidden');
      customMessageEl.innerHTML = '';
    }

    if (methods.includes('openid')) {
      oidcBtn.classList.remove('hidden');
      divider.classList.remove('hidden');

      const oidcBtnText = document.getElementById('oidc-btn-text');
      if (oidcBtnText && status.authFormData && status.authFormData.authOpenIDButtonText) {
        oidcBtnText.textContent = status.authFormData.authOpenIDButtonText;
      } else if (oidcBtnText) {
        oidcBtnText.textContent = 'Sign in with OpenId';
      }

      const triggerOidcRedirect = () => {
        const callbackUrl = encodeURIComponent(window.location.origin + window.location.pathname);
        window.location.href = resolvePath(`/auth/openid?redirect=${callbackUrl}`);
      };

      oidcBtn.onclick = triggerOidcRedirect;

      // Auto-Launch OIDC if configured (and not bypassed via ?local=1 or ?bypass=1)
      if (status.authFormData && status.authFormData.authOpenIDAutoLaunch) {
        const urlParams = new URLSearchParams(window.location.search);
        if (!urlParams.has('local') && !urlParams.has('bypass')) {
          triggerOidcRedirect();
          return;
        }
      }
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
